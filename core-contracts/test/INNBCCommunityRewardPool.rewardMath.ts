/* START ALPHA CHANGE 3870 -- regress full-precision community reward arithmetic and final range guards -- */

import { expect } from "chai";
import { BigNumber, Contract } from "ethers";
import { ethers, network } from "hardhat";
import {
  financeFixture,
  mined,
  resetAuditChain,
} from "./helpers/innbcAudit";

const PRECISION = BigNumber.from(10).pow(18);
const MAX_UINT256 = ethers.constants.MaxUint256;
const LAST_SAFE_PRODUCT = MAX_UINT256.div(PRECISION);

const TRANSACTION = {
  gasPrice: 0,
  gasLimit: 1_000_000,
};

async function communityRewardMathFixture() {
  const fixture = await financeFixture();
  const alice = fixture.accounts[3];
  const bob = fixture.accounts[4];
  const donor = fixture.outsider;

  // Fund test signers only. Pool storage is changed through its normal
  // stake, deposit, claim and unstake operations.
  const funding = ethers.utils.hexValue(BigNumber.from(2).pow(220));

  for (const signer of [alice, bob, donor]) {
    await network.provider.send("hardhat_setBalance", [
      signer.address,
      funding,
    ]);
  }

  return { ...fixture, alice, bob, donor };
}

async function communityRewardMathState(
  pool: Contract,
  account: string
) {
  return {
    poolBalance: (await ethers.provider.getBalance(pool.address)).toString(),
    accountBalance: (await ethers.provider.getBalance(account)).toString(),
    totalStaked: (await pool.totalStaked()).toString(),
    rewardIndex: (await pool.rewardPerTokenStored()).toString(),
    accountedRewards: (await pool.accountedRewardBalance()).toString(),
    stake: (await pool.stakedBalance(account)).toString(),
    pending: (await pool.pendingRewards(account)).toString(),
    paidIndex: (await pool.userRewardPerTokenPaid(account)).toString(),
  };
}

describe("INNBC community rewards: full-precision arithmetic", function () {
  beforeEach(resetAuditChain);

  const cases = [
    {
      name: "ordinary amounts preserve rounding dust",
      stake: BigNumber.from(3),
      rewards: BigNumber.from(10),
      productOverflows: false,
    },
    {
      name: "last reward whose intermediate product fits uint256",
      stake: LAST_SAFE_PRODUCT,
      rewards: LAST_SAFE_PRODUCT,
      productOverflows: false,
    },
    {
      name: "first reward whose intermediate product exceeds uint256",
      stake: LAST_SAFE_PRODUCT.add(1),
      rewards: LAST_SAFE_PRODUCT.add(1),
      productOverflows: true,
    },
    {
      name: "stake and rewards of two to the power of 199",
      stake: BigNumber.from(2).pow(199),
      rewards: BigNumber.from(2).pow(199),
      productOverflows: true,
    },
  ];

  for (const tc of cases) {
    it(tc.name, async function () {
      const f = await communityRewardMathFixture();
      const balanceBefore = await ethers.provider.getBalance(f.alice.address);

      expect(await f.pool.REWARD_PRECISION()).to.equal(PRECISION);

      await mined(
        f.pool.connect(f.alice).stake({
          ...TRANSACTION,
          value: tc.stake,
        })
      );

      await mined(
        f.donor.sendTransaction({
          ...TRANSACTION,
          to: f.pool.address,
          value: tc.rewards,
        })
      );

      // ethers BigNumber keeps the full intermediate product.
      const product = tc.rewards.mul(PRECISION);
      const expectedIndex = product.div(tc.stake);
      const expectedReward = tc.stake
        .mul(expectedIndex)
        .div(PRECISION);
      const dust = tc.rewards.sub(expectedReward);

      expect(product.gt(MAX_UINT256)).to.equal(tc.productOverflows);
      expect(expectedIndex.lte(MAX_UINT256)).to.equal(true);
      expect(expectedReward.lte(MAX_UINT256)).to.equal(true);

      const beforeView = await communityRewardMathState(
        f.pool,
        f.alice.address
      );

      expect(await f.pool.earned(f.alice.address)).to.equal(expectedReward);

      expect(
        await communityRewardMathState(f.pool, f.alice.address)
      ).to.deep.equal(beforeView);

      await expect(
        f.pool.connect(f.alice).claim(TRANSACTION)
      )
        .to.emit(f.pool, "RewardClaimed")
        .withArgs(f.alice.address, expectedReward);

      expect(await f.pool.rewardPerTokenStored()).to.equal(expectedIndex);
      expect(await f.pool.userRewardPerTokenPaid(f.alice.address)).to.equal(
        expectedIndex
      );
      expect(await f.pool.pendingRewards(f.alice.address)).to.equal(0);
      expect(await f.pool.earned(f.alice.address)).to.equal(0);
      expect(await f.pool.accountedRewardBalance()).to.equal(dust);
      expect(await f.pool.totalStaked()).to.equal(tc.stake);
      expect(await f.pool.stakedBalance(f.alice.address)).to.equal(tc.stake);
      expect(await ethers.provider.getBalance(f.pool.address)).to.equal(
        tc.stake.add(dust)
      );
      expect(await ethers.provider.getBalance(f.alice.address)).to.equal(
        balanceBefore.sub(tc.stake).add(expectedReward)
      );

      await expect(
        f.pool.connect(f.alice).unstake(tc.stake, TRANSACTION)
      )
        .to.emit(f.pool, "Unstaked")
        .withArgs(f.alice.address, tc.stake);

      expect(await f.pool.totalStaked()).to.equal(0);
      expect(await f.pool.stakedBalance(f.alice.address)).to.equal(0);
      expect(await f.pool.pendingRewards(f.alice.address)).to.equal(0);
      expect(await f.pool.earned(f.alice.address)).to.equal(0);
      expect(await f.pool.accountedRewardBalance()).to.equal(dust);
      expect(await ethers.provider.getBalance(f.pool.address)).to.equal(dust);
      expect(await ethers.provider.getBalance(f.alice.address)).to.equal(
        balanceBefore.add(expectedReward)
      );
    });
  }

  it("returns large principal before the outstanding reward is claimed", async function () {
    const f = await communityRewardMathFixture();
    const amount = BigNumber.from(2).pow(199);
    const balanceBefore = await ethers.provider.getBalance(f.alice.address);

    await mined(
      f.pool.connect(f.alice).stake({
        ...TRANSACTION,
        value: amount,
      })
    );

    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: amount,
      })
    );

    await expect(
      f.pool.connect(f.alice).unstake(amount, TRANSACTION)
    )
      .to.emit(f.pool, "Unstaked")
      .withArgs(f.alice.address, amount);

    expect(await f.pool.totalStaked()).to.equal(0);
    expect(await f.pool.stakedBalance(f.alice.address)).to.equal(0);
    expect(await f.pool.rewardPerTokenStored()).to.equal(PRECISION);
    expect(await f.pool.pendingRewards(f.alice.address)).to.equal(amount);
    expect(await f.pool.accountedRewardBalance()).to.equal(amount);
    expect(await f.pool.earned(f.alice.address)).to.equal(amount);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(amount);
    expect(await ethers.provider.getBalance(f.alice.address)).to.equal(
      balanceBefore
    );

    await expect(
      f.pool.connect(f.alice).claim(TRANSACTION)
    )
      .to.emit(f.pool, "RewardClaimed")
      .withArgs(f.alice.address, amount);

    expect(await f.pool.pendingRewards(f.alice.address)).to.equal(0);
    expect(await f.pool.accountedRewardBalance()).to.equal(0);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(0);
    expect(await ethers.provider.getBalance(f.alice.address)).to.equal(
      balanceBefore.add(amount)
    );
  });

  it("handles an overflowing user product after individually safe synchronizations", async function () {
    const f = await communityRewardMathFixture();
    const stake = LAST_SAFE_PRODUCT;

    expect(stake.mul(PRECISION).lte(MAX_UINT256)).to.equal(true);

    await mined(
      f.pool.connect(f.alice).stake({
        ...TRANSACTION,
        value: stake,
      })
    );
    await mined(
      f.pool.connect(f.bob).stake({
        ...TRANSACTION,
        value: 1,
      })
    );

    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: stake,
      })
    );
    await mined(
      f.pool.connect(f.bob).stake({
        ...TRANSACTION,
        value: 1,
      })
    );

    const firstIncrement = stake
      .mul(PRECISION)
      .div(stake.add(1));

    expect(firstIncrement).to.equal(PRECISION.sub(1));
    expect(await f.pool.rewardPerTokenStored()).to.equal(firstIncrement);

    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: stake,
      })
    );
    await mined(
      f.pool.connect(f.bob).stake({
        ...TRANSACTION,
        value: 1,
      })
    );

    const secondIncrement = stake
      .mul(PRECISION)
      .div(stake.add(2));
    const expectedIndex = firstIncrement.add(secondIncrement);

    expect(secondIncrement).to.equal(PRECISION.sub(1));
    expect(await f.pool.rewardPerTokenStored()).to.equal(expectedIndex);
    expect(await f.pool.accountedRewardBalance()).to.equal(stake.mul(2));
    expect(await f.pool.totalStaked()).to.equal(stake.add(3));

    const aliceProduct = stake.mul(expectedIndex);
    const aliceReward = aliceProduct.div(PRECISION);
    const bobReward = firstIncrement
      .div(PRECISION)
      .add(secondIncrement.mul(2).div(PRECISION));

    expect(aliceProduct.gt(MAX_UINT256)).to.equal(true);
    expect(aliceReward.lte(MAX_UINT256)).to.equal(true);
    expect(bobReward).to.equal(1);
    expect(await f.pool.pendingRewards(f.bob.address)).to.equal(bobReward);

    const beforeView = await communityRewardMathState(
      f.pool,
      f.alice.address
    );

    expect(await f.pool.earned(f.alice.address)).to.equal(aliceReward);
    expect(await f.pool.earned(f.bob.address)).to.equal(bobReward);

    expect(
      await communityRewardMathState(f.pool, f.alice.address)
    ).to.deep.equal(beforeView);

    const aliceBeforeClaim = await ethers.provider.getBalance(f.alice.address);

    await expect(
      f.pool.connect(f.alice).claim(TRANSACTION)
    )
      .to.emit(f.pool, "RewardClaimed")
      .withArgs(f.alice.address, aliceReward);

    expect(await ethers.provider.getBalance(f.alice.address)).to.equal(
      aliceBeforeClaim.add(aliceReward)
    );

    await expect(
      f.pool.connect(f.bob).claim(TRANSACTION)
    )
      .to.emit(f.pool, "RewardClaimed")
      .withArgs(f.bob.address, bobReward);

    await mined(f.pool.connect(f.alice).unstake(stake, TRANSACTION));
    await mined(f.pool.connect(f.bob).unstake(3, TRANSACTION));

    const dust = stake.mul(2).sub(aliceReward).sub(bobReward);

    expect(await f.pool.totalStaked()).to.equal(0);
    expect(await f.pool.stakedBalance(f.alice.address)).to.equal(0);
    expect(await f.pool.stakedBalance(f.bob.address)).to.equal(0);
    expect(await f.pool.pendingRewards(f.alice.address)).to.equal(0);
    expect(await f.pool.pendingRewards(f.bob.address)).to.equal(0);
    expect(await f.pool.accountedRewardBalance()).to.equal(dust);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(dust);
  });

  it("rejects a genuinely unrepresentable final index and preserves state", async function () {
    const f = await communityRewardMathFixture();
    const rewards = BigNumber.from(2).pow(200);

    await mined(
      f.pool.connect(f.alice).stake({
        ...TRANSACTION,
        value: 1,
      })
    );
    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: rewards,
      })
    );

    expect(rewards.mul(PRECISION).gt(MAX_UINT256)).to.equal(true);

    const before = await communityRewardMathState(f.pool, f.alice.address);

    await expect(
      f.pool.earned(f.alice.address)
    ).to.be.revertedWith("Math: mulDiv overflow");

    await expect(
      f.pool.connect(f.alice).claim(TRANSACTION)
    ).to.be.revertedWith("Math: mulDiv overflow");

    expect(
      await communityRewardMathState(f.pool, f.alice.address)
    ).to.deep.equal(before);

    await expect(
      f.pool.connect(f.alice).unstake(1, TRANSACTION)
    ).to.be.revertedWith("Math: mulDiv overflow");

    expect(
      await communityRewardMathState(f.pool, f.alice.address)
    ).to.deep.equal(before);
  });

  it("keeps cumulative reward-index overflow checked and atomic", async function () {
    const f = await communityRewardMathFixture();
    const firstReward = LAST_SAFE_PRODUCT;

    await mined(
      f.pool.connect(f.alice).stake({
        ...TRANSACTION,
        value: 1,
      })
    );
    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: firstReward,
      })
    );
    await mined(f.pool.connect(f.alice).claim(TRANSACTION));

    const previousIndex = firstReward.mul(PRECISION);

    expect(previousIndex.lte(MAX_UINT256)).to.equal(true);
    expect(previousIndex.add(PRECISION).gt(MAX_UINT256)).to.equal(true);
    expect(await f.pool.rewardPerTokenStored()).to.equal(previousIndex);
    expect(await f.pool.accountedRewardBalance()).to.equal(0);
    expect(await ethers.provider.getBalance(f.pool.address)).to.equal(1);

    await mined(
      f.donor.sendTransaction({
        ...TRANSACTION,
        to: f.pool.address,
        value: 1,
      })
    );

    const before = await communityRewardMathState(f.pool, f.alice.address);

    await expect(
      f.pool.earned(f.alice.address)
    ).to.be.revertedWithPanic(0x11);

    await expect(
      f.pool.connect(f.alice).claim(TRANSACTION)
    ).to.be.revertedWithPanic(0x11);

    expect(
      await communityRewardMathState(f.pool, f.alice.address)
    ).to.deep.equal(before);

    await expect(
      f.pool.connect(f.alice).unstake(1, TRANSACTION)
    ).to.be.revertedWithPanic(0x11);

    expect(
      await communityRewardMathState(f.pool, f.alice.address)
    ).to.deep.equal(before);
  });
});

/* FINISH ALPHA CHANGE 3870 -- regress full-precision community reward arithmetic and final range guards -- */