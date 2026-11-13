/* START ALPHA CHANGE 3786 -- test emergency reward multiplication boundaries and supply overflow rollback -- */
import { expect } from "chai";
import { BigNumber } from "ethers";
import { ethers } from "hardhat";

const MAX_UINT256 = ethers.constants.MaxUint256;

async function deployRewardMathFixture(initialSupply: BigNumber) {
  const [validator1, validator2, reward1, reward2] =
    await ethers.getSigners();

  const Registry = await ethers.getContractFactory(
    "MockINNBCValidatorRegistry"
  );
  const registry = await Registry.deploy();
  await registry.deployed();

  const Token = await ethers.getContractFactory(
    "MockINNBCBlockRewardNativeToken"
  );
  const token = await Token.deploy(initialSupply);
  await token.deployed();

  const CommunityPool = await ethers.getContractFactory(
    "MockINNBCBlockRewardCommunityRewardPool"
  );
  const communityPool = await CommunityPool.deploy();
  await communityPool.deployed();

  const Governance = await ethers.getContractFactory(
    "INNBCBlockRewardGovernance"
  );
  const governance = await Governance.deploy();
  await governance.deployed();

  await (
    await governance.initialize(
      registry.address,
      token.address,
      communityPool.address
    )
  ).wait();

  await (
    await registry.addActiveValidator(
      validator1.address,
      reward1.address,
      1
    )
  ).wait();

  await (
    await registry.addActiveValidator(
      validator2.address,
      reward2.address,
      1
    )
  ).wait();

  return {
    governance,
    token,
    validator1,
    validator2,
    reward1,
    reward2,
  };
}

describe("INNBC emergency rewards: full-precision arithmetic", function () {
  /* START ALPHA CHANGE 3787 -- preserve literal method names for typed governance calls -- */
  const rates = [
    {
      bps: 100,
      create: "createOnePercentEmergencyRewardProposal",
    },
    {
      bps: 200,
      create: "createTwoPercentEmergencyRewardProposal",
    },
    {
      bps: 300,
      create: "createThreePercentEmergencyRewardProposal",
    },
  ] as const;
  /* FINISH ALPHA CHANGE 3787 -- preserve literal method names for typed governance calls -- */

  for (const rate of rates) {
    const lastSafeProductSupply = MAX_UINT256.div(rate.bps);

    const cases = [
      {
        name: "ordinary supply and rounding",
        supply: BigNumber.from(10_050),
      },
      {
        name: "last supply whose intermediate product fits uint256",
        supply: lastSafeProductSupply,
      },
      {
        name: "first supply whose intermediate product exceeds uint256",
        supply: lastSafeProductSupply.add(1),
      },
      {
        name: "reported supply of two to the power of 255",
        supply: BigNumber.from(2).pow(255),
      },
      {
        name: "final supply would exceed uint256",
        supply: MAX_UINT256,
      },
    ];

    for (const tc of cases) {
      it(`${rate.bps / 100}%: ${tc.name}`, async function () {
        const f = await deployRewardMathFixture(tc.supply);
        const id = await f.governance.nextProposalID();

        await (await f.governance[rate.create]()).wait();
        await (
          await f.governance.connect(f.validator1).vote(id, true)
        ).wait();
        await (
          await f.governance.connect(f.validator2).vote(id, true)
        ).wait();

        // ethers BigNumber arithmetic is not restricted to uint256.
        // Calculate the mathematical result, including both rounding stages.
        const perValidator = tc.supply
          .mul(rate.bps)
          .div(10_000)
          .div(2);
        const issued = perValidator.mul(2);
        const expectedSupply = tc.supply.add(issued);

        if (expectedSupply.gt(MAX_UINT256)) {
          // Full-precision reward calculation must not bypass the token's
          // checked addition when actual issuance exceeds its supply range.
          await expect(
            f.governance.finalizeProposal(id)
          ).to.be.revertedWithPanic(0x11);

          expect(await f.token.totalSupply()).to.equal(tc.supply);
          expect(await f.token.balanceOf(f.reward1.address)).to.equal(0);
          expect(await f.token.balanceOf(f.reward2.address)).to.equal(0);

          const record = await f.governance.proposals(id);
          expect(record.finalized).to.equal(false);
          expect(record.passed).to.equal(false);

          expect(
            await f.governance.lastEmergencyRewardBlock()
          ).to.equal(0);
          expect(
            await f.governance.emergencyRewardProposalActive()
          ).to.equal(true);
          expect(
            await f.governance.getActiveProposalIDs()
          ).to.deep.equal([id]);

          return;
        }

        const finalization = await f.governance.finalizeProposal(id);
        const receipt = await finalization.wait();

        await expect(finalization)
          .to.emit(f.governance, "EmergencyRewardDistributed")
          .withArgs(id, issued, perValidator, rate.bps);

        expect(await f.token.totalSupply()).to.equal(expectedSupply);
        expect(
          await f.token.balanceOf(f.reward1.address)
        ).to.equal(perValidator);
        expect(
          await f.token.balanceOf(f.reward2.address)
        ).to.equal(perValidator);

        const record = await f.governance.proposals(id);
        expect(record.finalized).to.equal(true);
        expect(record.passed).to.equal(true);

        expect(
          await f.governance.lastEmergencyRewardBlock()
        ).to.equal(receipt.blockNumber);
        expect(
          await f.governance.emergencyRewardProposalActive()
        ).to.equal(false);
        expect(
          await f.governance.getActiveProposalIDs()
        ).to.deep.equal([]);
      });
    }
  }
});
/* FINISH ALPHA CHANGE 3786 -- test emergency reward multiplication boundaries and supply overflow rollback -- */