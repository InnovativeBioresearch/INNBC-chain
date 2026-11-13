/* START ALPHA CHANGE 2921 -- verify retired BLS getters disappear from contract artifacts and factories -- */
import { expect } from "chai";
import { artifacts } from "hardhat";
/* START ALPHA CHANGE 3757 -- remove the retired AccessList factory dependency -- */
import {
  INNBCNativeToken__factory,
  System__factory,
  ValidatorSet__factory,
} from "../typechain-types";
/* FINISH ALPHA CHANGE 3757 -- remove the retired AccessList factory dependency -- */

type ABIEntry = {
  type: string;
  name?: string;
};

describe("INNBC audit: aggregate BLS precompile retirement", function () {
  /* START ALPHA CHANGE 3758 -- retain BLS getter regressions for the three current contracts -- */
  const contracts: Array<{
    name: string;
    factoryABI: ReadonlyArray<ABIEntry>;
  }> = [
    {
      name: "System",
      factoryABI: System__factory.abi,
    },
    {
      name: "INNBCNativeToken",
      factoryABI: INNBCNativeToken__factory.abi,
    },
    {
      name: "ValidatorSet",
      factoryABI: ValidatorSet__factory.abi,
    },
  ];
  /* FINISH ALPHA CHANGE 3758 -- retain BLS getter regressions for the three current contracts -- */

  for (const entry of contracts) {
    it(`${entry.name}: removes legacy getters and preserves native-transfer getters`, async function () {
      const artifact = await artifacts.readArtifact(entry.name);

      const sources: Array<{
        name: string;
        abi: ReadonlyArray<ABIEntry>;
      }> = [
        {
          name: `${entry.name} Hardhat artifact`,
          abi: artifact.abi,
        },
        {
          name: `${entry.name} TypeChain factory`,
          abi: entry.factoryABI,
        },
      ];

      for (const source of sources) {
        const names = source.abi
          .filter((item) => item.type === "function")
          .map((item) => item.name);

        expect(names, source.name).not.to.include(
          "VALIDATOR_PKCHECK_PRECOMPILE"
        );
        expect(names, source.name).not.to.include(
          "VALIDATOR_PKCHECK_PRECOMPILE_GAS"
        );
        expect(names, source.name).to.include(
          "NATIVE_TRANSFER_PRECOMPILE"
        );
        expect(names, source.name).to.include(
          "NATIVE_TRANSFER_PRECOMPILE_GAS"
        );
      }
    });
  }
});
/* FINISH ALPHA CHANGE 2921 -- verify retired BLS getters disappear from contract artifacts and factories -- */