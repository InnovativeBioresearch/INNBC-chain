/* START ALPHA CHANGE 3428 -- verify compiled ValidatorSet artifacts and initialization guards -- */
import { expect } from "chai";
import { artifacts, ethers, network } from "hardhat";
import {
  IValidatorSet__factory,
  ValidatorSet__factory,
} from "../typechain-types";
import {
  deploy,
  installRuntime,
  mined,
  resetAuditChain,
} from "./helpers/innbcAudit";

type ABIEntry = {
  type: string;
  name?: string;
  inputs?: ReadonlyArray<{ type: string }>;
};

describe("INNBC audit: ValidatorSet epoch-only retirement", function () {
  const retiredMethods = [
    "name",
    "symbol",
    "decimals",
    "totalSupply",
    "balanceOf",
    "transfer",
    "allowance",
    "approve",
    "transferFrom",
    "increaseAllowance",
    "decreaseAllowance",
    "balanceOfAt",
    "totalSupplyAt",
    "unstake",
    "withdraw",
    "withdrawable",
    "pendingWithdrawals",
    "WITHDRAWAL_WAIT_PERIOD",
  ];

  const contracts: Array<{
    name: string;
    factoryABI: ReadonlyArray<ABIEntry>;
  }> = [
    {
      name: "ValidatorSet",
      factoryABI: ValidatorSet__factory.abi,
    },
    {
      name: "IValidatorSet",
      factoryABI: IValidatorSet__factory.abi,
    },
  ];

  for (const entry of contracts) {
    it(`${entry.name}: exposes epoch operations without VSET accounting`, async function () {
      const artifact = await artifacts.readArtifact(entry.name);

      for (const source of [
        { name: "Hardhat", abi: artifact.abi as ReadonlyArray<ABIEntry> },
        { name: "TypeChain", abi: entry.factoryABI },
      ]) {
        const methods = source.abi
          .filter((item) => item.type === "function")
          .map((item) => item.name);
        const events = source.abi
          .filter((item) => item.type === "event")
          .map((item) => item.name);

        for (const name of retiredMethods) {
          expect(methods, source.name).not.to.include(name);
        }

        for (const name of ["commitEpoch", "EPOCH_SIZE", "totalBlocks"]) {
          expect(methods, source.name).to.include(name);
        }

        expect(events, source.name).to.include("NewEpoch");
        for (const name of [
          "Transfer",
          "Approval",
          "Snapshot",
          "WithdrawalRegistered",
          "Withdrawal",
        ]) {
          expect(events, source.name).not.to.include(name);
        }

        if (entry.name === "ValidatorSet") {
          const initializer = source.abi.find(
            (item) => item.type === "function" && item.name === "initialize"
          );
          expect(initializer, source.name).not.to.equal(undefined);
          expect(
            initializer!.inputs!.map((input) => input.type),
            source.name
          ).to.deep.equal(["uint256"]);

          expect(methods, source.name).to.include("currentEpochId");
          expect(methods, source.name).to.include("epochs");
          expect(methods, source.name).to.include("epochEndBlocks");
        }
      }
    });
  }

  it("locks conventional deployments and protects runtime allocations", async function () {
    await resetAuditChain();

    const systemAddress = ethers.utils.getAddress(
      "0xfffffffffffffffffffffffffffffffffffffffe"
    );
    await network.provider.send("hardhat_setBalance", [
      systemAddress,
      "0x1000000000000000000",
    ]);
    await network.provider.send("hardhat_impersonateAccount", [
      systemAddress,
    ]);
    const system = await ethers.getSigner(systemAddress);

    try {
      const conventional = await deploy("ValidatorSet");
      await expect(
        conventional.connect(system).initialize(2)
      ).to.be.revertedWith("Initializable: contract is already initialized");

      const allocated = await installRuntime(
        "ValidatorSet",
        "0x0000000000000000000000000000000000001011"
      );

      await expect(
        allocated.initialize(2)
      ).to.be.revertedWithCustomError(allocated, "Unauthorized")
        .withArgs("SYSTEMCALL");

      /* START ALPHA CHANGE 3447 -- verify the epoch minimum and initialization rollback -- */
      for (const epochSize of [0, 1]) {
        await expect(
          allocated.connect(system).initialize(epochSize)
        ).to.be.revertedWith("INVALID_INPUT");

        expect(await allocated.EPOCH_SIZE()).to.equal(0);
        expect(await allocated.currentEpochId()).to.equal(0);
      }

      await mined(allocated.connect(system).initialize(2));
      expect(await allocated.EPOCH_SIZE()).to.equal(2);
      expect(await allocated.currentEpochId()).to.equal(1);
      expect(await allocated.epochEndBlocks(0)).to.equal(0);
      /* FINISH ALPHA CHANGE 3447 -- verify the epoch minimum and initialization rollback -- */

      await expect(
        allocated.connect(system).initialize(3)
      ).to.be.revertedWith("Initializable: contract is already initialized");
    } finally {
      await network.provider.send("hardhat_stopImpersonatingAccount", [
        systemAddress,
      ]);
    }
  });
});
/* FINISH ALPHA CHANGE 3428 -- verify compiled ValidatorSet artifacts and initialization guards -- */