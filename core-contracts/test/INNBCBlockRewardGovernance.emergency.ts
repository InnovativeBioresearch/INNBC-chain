import { expect } from "chai";
import { ethers } from "hardhat";

// START ALPHA CHANGE 979 -- add emergency block reward cooldown governance test
describe("INNBCBlockRewardGovernance emergency rewards", function () {
  async function deployFixture() {
    const [validator1, validator2, reward1, reward2] = await ethers.getSigners();

    const Registry = await ethers.getContractFactory("MockINNBCValidatorRegistry");
    const registry = await Registry.deploy();
    await registry.deployed();

    const Token = await ethers.getContractFactory("MockINNBCBlockRewardNativeToken");
    /* START ALPHA CHANGE 3782 -- distinguish updated supply and issuance rounding across emergency cycles -- */
    const token = await Token.deploy(10_050);
    /* FINISH ALPHA CHANGE 3782 -- distinguish updated supply and issuance rounding across emergency cycles -- */
    await token.deployed();

    const CommunityPool = await ethers.getContractFactory("MockINNBCBlockRewardCommunityRewardPool");
    const communityPool = await CommunityPool.deploy();
    await communityPool.deployed();

    const Governance = await ethers.getContractFactory("INNBCBlockRewardGovernance");
    const governance = await Governance.deploy();
    await governance.deployed();

    await governance.initialize(registry.address, token.address, communityPool.address);
    await registry.addActiveValidator(validator1.address, reward1.address, 1);
    await registry.addActiveValidator(validator2.address, reward2.address, 1);

    return { governance, registry, token, validator1, validator2, reward1, reward2 };
  }

  /* START ALPHA CHANGE 3783 -- test repeated emergency issuance and exact cooldown boundaries -- */
  it("enforces exact cooldown boundaries across successive 2% and 3% issuances", async function () {
    const {
      governance,
      token,
      validator1,
      validator2,
      reward1,
      reward2,
    } = await deployFixture();

    const cooldownError =
      "INNBC Block Reward: emergency reward cooldown active";
    const cooldown = (
      await governance.EMERGENCY_REWARD_COOLDOWN_BLOCKS()
    ).toNumber();

    async function mineTo(target: number): Promise<void> {
      const current = await ethers.provider.getBlockNumber();
      expect(current).to.be.at.most(target);

      if (current < target) {
        await ethers.provider.send("hardhat_mine", [
          ethers.utils.hexValue(target - current),
        ]);
      }

      expect(await ethers.provider.getBlockNumber()).to.equal(target);
    }

    expect(await token.totalSupply()).to.equal(10_050);

    // First issuance: floor(10,050 * 2%) = 201.
    // Two validators receive 100 each; only 200 tokens are issued.
    const firstID = await governance.nextProposalID();
    const firstCreationTx = await governance
      .connect(validator1)
      .createTwoPercentEmergencyRewardProposal();
    await firstCreationTx.wait();

    await expect(firstCreationTx).to.emit(governance, "ProposalCreated");

    expect(await governance.getActiveProposalIDs()).to.deep.equal([firstID]);
    expect(
      (await governance.proposals(firstID)).emergencyRewardRateBps
    ).to.equal(200);
    expect(await governance.emergencyRewardProposalActive()).to.equal(true);

    await expect(
      governance.connect(validator1).callStatic
        .createThreePercentEmergencyRewardProposal({ blockTag: "latest" })
    ).to.be.revertedWith(
      "INNBC Block Reward: emergency reward proposal active"
    );

    await (await governance.connect(validator1).vote(firstID, true)).wait();
    await (await governance.connect(validator2).vote(firstID, true)).wait();

    const firstFinalizationTx = await governance.finalizeProposal(firstID);
    const firstReceipt = await firstFinalizationTx.wait();

    await expect(firstFinalizationTx)
      .to.emit(governance, "EmergencyRewardDistributed")
      .withArgs(firstID, 200, 100, 200);

    expect(await token.totalSupply()).to.equal(10_250);
    expect(await token.balanceOf(reward1.address)).to.equal(100);
    expect(await token.balanceOf(reward2.address)).to.equal(100);

    const firstFinalized = await governance.proposals(firstID);
    expect(firstFinalized.finalized).to.equal(true);
    expect(firstFinalized.passed).to.equal(true);
    expect(await governance.lastEmergencyRewardBlock()).to.equal(
      firstReceipt.blockNumber
    );
    expect(await governance.emergencyRewardProposalActive()).to.equal(false);
    expect(await governance.getActiveProposalIDs()).to.deep.equal([]);

    await expect(
      governance.connect(validator1).callStatic
        .createThreePercentEmergencyRewardProposal({ blockTag: "latest" })
    ).to.be.revertedWith(cooldownError);

    // Reject at T - 1 without mining a transaction.
    const firstAllowedBlock = firstReceipt.blockNumber + cooldown;
    await mineTo(firstAllowedBlock - 1);

    await expect(
      governance.connect(validator1).callStatic
        .createThreePercentEmergencyRewardProposal({ blockTag: "latest" })
    ).to.be.revertedWith(cooldownError);

    expect(await ethers.provider.getBlockNumber()).to.equal(
      firstAllowedBlock - 1
    );

    const secondID = await governance.nextProposalID();
    expect(secondID).to.equal(firstID.add(1));

    // Explicit gas avoids estimation against the preceding blocked height.
    // No state-changing transaction occurs between the boundary check above
    // and this proposal-creation transaction.
    const secondCreationTx = await governance
      .connect(validator1)
      .createThreePercentEmergencyRewardProposal({ gasLimit: 1_000_000 });
    const secondCreationReceipt = await secondCreationTx.wait();

    expect(secondCreationReceipt.blockNumber).to.equal(firstAllowedBlock);
    await expect(secondCreationTx).to.emit(governance, "ProposalCreated");

    expect(await governance.getActiveProposalIDs()).to.deep.equal([secondID]);
    expect(
      (await governance.proposals(secondID)).emergencyRewardRateBps
    ).to.equal(300);
    expect(await governance.emergencyRewardProposalActive()).to.equal(true);

    // Proposal creation must not restart the issuance cooldown.
    expect(await governance.lastEmergencyRewardBlock()).to.equal(
      firstReceipt.blockNumber
    );

    const supplyBeforeSecond = await token.totalSupply();
    const reward1BeforeSecond = await token.balanceOf(reward1.address);
    const reward2BeforeSecond = await token.balanceOf(reward2.address);

    await (await governance.connect(validator1).vote(secondID, true)).wait();
    await (await governance.connect(validator2).vote(secondID, true)).wait();

    // Updated supply: floor(10,250 * 3%) = 307.
    // Two validators receive 153 each; only 306 tokens are issued.
    // Using the stale initial supply would incorrectly give 150 each.
    const secondFinalizationTx = await governance.finalizeProposal(secondID);
    const secondReceipt = await secondFinalizationTx.wait();

    await expect(secondFinalizationTx)
      .to.emit(governance, "EmergencyRewardDistributed")
      .withArgs(secondID, 306, 153, 300);

    const supplyAfterSecond = await token.totalSupply();
    const reward1AfterSecond = await token.balanceOf(reward1.address);
    const reward2AfterSecond = await token.balanceOf(reward2.address);

    expect(supplyAfterSecond.sub(supplyBeforeSecond)).to.equal(306);
    expect(supplyAfterSecond).to.equal(10_556);
    expect(reward1AfterSecond.sub(reward1BeforeSecond)).to.equal(153);
    expect(reward2AfterSecond.sub(reward2BeforeSecond)).to.equal(153);
    expect(reward1AfterSecond).to.equal(253);
    expect(reward2AfterSecond).to.equal(253);

    const secondFinalized = await governance.proposals(secondID);
    expect(secondFinalized.finalized).to.equal(true);
    expect(secondFinalized.passed).to.equal(true);
    expect(await governance.emergencyRewardProposalActive()).to.equal(false);
    expect(await governance.getActiveProposalIDs()).to.deep.equal([]);

    expect(secondReceipt.blockNumber).to.be.greaterThan(
      secondCreationReceipt.blockNumber
    );
    expect(await governance.lastEmergencyRewardBlock()).to.equal(
      secondReceipt.blockNumber
    );

    // A successful second issuance starts a fresh cooldown.
    await expect(
      governance.connect(validator1).callStatic
        .createOnePercentEmergencyRewardProposal({ blockTag: "latest" })
    ).to.be.revertedWith(cooldownError);

    const secondAllowedBlock = secondReceipt.blockNumber + cooldown;
    await mineTo(secondAllowedBlock - 1);

    await expect(
      governance.connect(validator1).callStatic
        .createOnePercentEmergencyRewardProposal({ blockTag: "latest" })
    ).to.be.revertedWith(cooldownError);

    expect(await ethers.provider.getBlockNumber()).to.equal(
      secondAllowedBlock - 1
    );
    expect(await governance.lastEmergencyRewardBlock()).to.equal(
      secondReceipt.blockNumber
    );
    expect(await governance.emergencyRewardProposalActive()).to.equal(false);
    expect(await governance.getActiveProposalIDs()).to.deep.equal([]);
  });
  /* FINISH ALPHA CHANGE 3783 -- test repeated emergency issuance and exact cooldown boundaries -- */
});
// FINISH ALPHA CHANGE 979 -- add emergency block reward cooldown governance test