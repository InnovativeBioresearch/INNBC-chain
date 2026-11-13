/* START ALPHA CHANGE 2399 -- share isolated INNBC contract audit fixtures -- */
import { expect } from "chai";
import { artifacts, ethers, network } from "hardhat";
import { BigNumberish, Contract, ContractTransaction } from "ethers";

export const ZERO = ethers.constants.AddressZero;
export const BLS_ADDRESS = ethers.utils.getAddress(
  "0x0000000000000000000000000000000000000102"
);
export const REGISTRY_PROXY = ethers.utils.getAddress(
  "0x0000000000000000000000000000000000000107"
);

export async function resetAuditChain(): Promise<void> {
  if (network.name !== "hardhat") {
    throw new Error("Contract audit tests require the local hardhat network");
  }

  await network.provider.send("hardhat_reset");
}

export async function mined(
  transaction: Promise<ContractTransaction>
): Promise<void> {
  await (await transaction).wait();
}

export async function deploy(
  name: string,
  ...args: any[]
): Promise<Contract> {
  const factory = await ethers.getContractFactory(name);
  const contract = await factory.deploy(...args);
  await contract.deployed();
  return contract;
}

export async function installRuntime(
  name: string,
  address: string
): Promise<Contract> {
  const artifact = await artifacts.readArtifact(name);
  await network.provider.send("hardhat_setCode", [
    address,
    artifact.deployedBytecode,
  ]);
  return ethers.getContractAt(name, address);
}

export function keyBytes(seed: number): string {
  return ethers.utils.hexlify(new Uint8Array(128).fill(seed));
}

export const PROOF = ethers.utils.hexlify(new Uint8Array(64).fill(7));

export async function proposal(
  contract: Contract,
  method: string,
  args: any[] = []
): Promise<any> {
  const id = await contract.nextProposalID();
  await mined(contract[method](...args));
  return id;
}

export async function approve(
  contract: Contract,
  id: BigNumberish,
  voters: any[]
): Promise<void> {
  for (const voter of voters) {
    await mined(contract.connect(voter).vote(id, true));
  }
}

export async function mineTo(block: number): Promise<void> {
  const current = await ethers.provider.getBlockNumber();
  if (block > current) {
    await network.provider.send("hardhat_mine", [
      ethers.utils.hexValue(block - current),
    ]);
  }
}

export async function registryFixture(count = 3) {
  const accounts = await ethers.getSigners();
  const validators = accounts.slice(0, count);
  const rewards = accounts.slice(6, 6 + count);
  const outsider = accounts[5];

  const bls = await installRuntime("INNBCAuditBLS", BLS_ADDRESS);
  await mined(bls.configure(true, true));

  const token = await deploy("INNBCAuditToken");
  const registry = await deploy("INNBCValidatorRegistry");
  const governance = await deploy("INNBCValidatorGovernance");

  await mined(governance.initialize(registry.address, 1));
  await mined(
    registry.initialize(
      validators[0].address,
      100,
      1,
      governance.address,
      token.address,
      2
    )
  );

  for (let i = 0; i < count; i++) {
    const validator = validators[i];
    await mined(token.mint(validator.address, 1000));
    await mined(token.connect(validator).approve(registry.address, 1000));
    await mined(
      registry.connect(validator).registerValidator(
        rewards[i].address,
        keyBytes(i + 1),
        PROOF,
        `audit-peer-${i}`,
        100
      )
    );

    if (i !== 0) {
      const id = await proposal(
        governance.connect(validators[0]),
        "createWhitelistProposal",
        [validator.address]
      );
      await approve(governance, id, validators.slice(0, i));
      await mined(governance.connect(outsider).finalizeProposal(id));
    }
  }

  expect(await registry.activeValidatorCount()).to.equal(count);

  return {
    accounts,
    validators,
    rewards,
    outsider,
    bls,
    token,
    registry,
    governance,
  };
}

/* START ALPHA CHANGE 3558 -- allow contract fixtures with different validator counts -- */
export async function financeFixture(count = 3) {
  const fixture = await registryFixture(count);
  /* FINISH ALPHA CHANGE 3558 -- allow contract fixtures with different validator counts -- */

  const pool = await deploy("INNBCCommunityRewardPool");
  const rewardsGovernance = await deploy("INNBCBlockRewardGovernance");
  const treasury = await deploy("INNBCTreasuryDAO");
  const vault = await deploy("INNBCFeeBurnVault");

  await mined(pool.initialize(fixture.registry.address, rewardsGovernance.address));
  await mined(
    rewardsGovernance.initialize(
      fixture.registry.address,
      fixture.token.address,
      pool.address
    )
  );
  await mined(treasury.initialize(fixture.registry.address, fixture.token.address));
  await mined(vault.initialize(fixture.token.address));

  return { ...fixture, pool, rewardsGovernance, treasury, vault };
}

/* FINISH ALPHA CHANGE 2399 -- share isolated INNBC contract audit fixtures -- */