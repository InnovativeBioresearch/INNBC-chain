/* START ALPHA CHANGE 3752 -- test max base fee bounds and finalization safety -- */
import { expect } from "chai";
import { artifacts, ethers, network } from "hardhat";
import {
  approve,
  financeFixture,
  mined,
  proposal,
  resetAuditChain,
} from "./helpers/innbcAudit";

const MAX_UINT64 = ethers.BigNumber.from("18446744073709551615");
const CAP_SLOT = ethers.utils.id(
  "innbc.blockRewardGovernance.maxBaseFeeWei"
);
const RANGE_ERROR = "INNBC Block Reward: max base fee exceeds uint64";

describe("INNBC max base fee: runtime range", function () {
  beforeEach(resetAuditChain);

  it("applies supported positive caps and allows zero to disable the cap", async function () {
    const f = await financeFixture();
    const governance = f.rewardsGovernance;

    expect(await governance.maxBaseFeeWei()).to.equal(0);

    const values = [
      ethers.BigNumber.from(1),
      ethers.BigNumber.from(123456),
      MAX_UINT64,
      ethers.BigNumber.from(0),
    ];

    for (const value of values) {
      const id = await proposal(
        governance,
        "createUpdateMaxBaseFeeProposal",
        [value]
      );

      await approve(governance, id, f.validators.slice(0, 2));
      await mined(governance.connect(f.outsider).finalizeProposal(id));

      const storedProposal = await governance.proposals(id);
      expect(storedProposal.finalized).to.equal(true);
      expect(storedProposal.passed).to.equal(true);
      expect(await governance.maxBaseFeeWei()).to.equal(value);

      const storedCap = await ethers.provider.getStorageAt(
        governance.address,
        CAP_SLOT
      );
      expect(ethers.BigNumber.from(storedCap)).to.equal(value);
      expect(await governance.getActiveProposalIDs()).to.deep.equal([]);
    }

    await expect(
      governance.createUpdateMaxBaseFeeProposal(0)
    ).to.be.revertedWith("INNBC Block Reward: max base fee unchanged");
  });

  it("rejects oversized proposals without changing proposal state or the cap", async function () {
    const f = await financeFixture();
    const governance = f.rewardsGovernance;

    const validID = await proposal(
      governance,
      "createUpdateMaxBaseFeeProposal",
      [100]
    );
    await approve(governance, validID, f.validators.slice(0, 2));
    await mined(governance.finalizeProposal(validID));

    const nextID = await governance.nextProposalID();
    const activeIDs = await governance.getActiveProposalIDs();

    for (const value of [
      MAX_UINT64.add(1),
      ethers.constants.MaxUint256,
    ]) {
      await expect(
        governance.createUpdateMaxBaseFeeProposal(value)
      ).to.be.revertedWith(RANGE_ERROR);

      expect(await governance.nextProposalID()).to.equal(nextID);
      expect(await governance.getActiveProposalIDs()).to.deep.equal(activeIDs);
      expect(await governance.maxBaseFeeWei()).to.equal(100);

      const storedCap = await ethers.provider.getStorageAt(
        governance.address,
        CAP_SLOT
      );
      expect(ethers.BigNumber.from(storedCap)).to.equal(100);
    }
  });

  it("rejects finalization of an oversized legacy pending proposal atomically", async function () {
    const f = await financeFixture();
    const governance = f.rewardsGovernance;

    const id = await proposal(
      governance,
      "createUpdateMaxBaseFeeProposal",
      [100]
    );
    await approve(governance, id, f.validators.slice(0, 2));

    // Simulate a pending proposal accepted by an older implementation.
    // Derive its storage location from compiler output instead of
    // hard-coding the proposal mapping or struct layout.
    const source =
      "contracts/innbc/INNBCBlockRewardGovernance.sol";
    const name = "INNBCBlockRewardGovernance";
    const buildInfo = await artifacts.getBuildInfo(`${source}:${name}`);

    if (!buildInfo) {
      throw new Error("Missing block reward governance build information");
    }

    const layout = (
      buildInfo.output.contracts[source][name] as any
    ).storageLayout;

    const mappingEntry = layout.storage.find(
      (entry: { label: string }) => entry.label === "proposals"
    );
    if (!mappingEntry) {
      throw new Error("Missing proposals mapping in storage layout");
    }

    const mappingType = layout.types[mappingEntry.type];
    const proposalType = layout.types[mappingType.value];
    const capMember = proposalType.members.find(
      (member: { label: string }) =>
        member.label === "newMaxBaseFeeWei"
    );
    if (!capMember || capMember.offset !== 0) {
      throw new Error("Unexpected max base fee proposal storage layout");
    }

    const mappingBase = ethers.BigNumber.from(
      ethers.utils.keccak256(
        ethers.utils.defaultAbiCoder.encode(
          ["uint256", "uint256"],
          [id, mappingEntry.slot]
        )
      )
    );
    const proposalCapSlot = mappingBase
      .add(capMember.slot)
      .mod(ethers.constants.MaxUint256.add(1));

    const oversizedValue = MAX_UINT64.add(1);
    await network.provider.send("hardhat_setStorageAt", [
      governance.address,
      ethers.utils.hexValue(proposalCapSlot),
      ethers.utils.hexZeroPad(oversizedValue.toHexString(), 32),
    ]);

    expect(
      (await governance.proposals(id)).newMaxBaseFeeWei
    ).to.equal(oversizedValue);

    await expect(
      governance.connect(f.outsider).finalizeProposal(id)
    ).to.be.revertedWith(RANGE_ERROR);

    const storedProposal = await governance.proposals(id);
    expect(storedProposal.finalized).to.equal(false);
    expect(storedProposal.passed).to.equal(false);
    expect(await governance.getActiveProposalIDs()).to.deep.equal([id]);
    expect(await governance.maxBaseFeeWei()).to.equal(0);

    const storedCap = await ethers.provider.getStorageAt(
      governance.address,
      CAP_SLOT
    );
    expect(ethers.BigNumber.from(storedCap)).to.equal(0);
  });
});
/* FINISH ALPHA CHANGE 3752 -- test max base fee bounds and finalization safety -- */