import { getAddress, type Address } from "viem";
import {
  assertHorizenChain,
  bundlerUrl,
  clients,
  deploymentEnv,
  deploymentPath,
  horizenTestnet,
  loadArtifact,
  normalizeAddress,
  privateKeyEnv,
  rpcUrl,
  saveDeployment,
  type AADeployment,
} from "./horizen-aa";

const privateKey = privateKeyEnv("HORIZEN_AA_DEPLOYER_PRIVATE_KEY");
const { publicClient, walletClient } = await clients(privateKey);

if (!walletClient?.account) {
  throw new Error("wallet client was not initialized");
}

await assertHorizenChain();

const deployer = getAddress(walletClient.account.address) as Address;
console.log(`Deploying Horizen AA contracts from ${deployer}`);
console.log(`RPC: ${rpcUrl()}`);

const existingEntryPoint = process.env.HORIZEN_AA_ENTRYPOINT_ADDRESS?.trim();
let entryPointAddress: Address;
let entryPointHash: `0x${string}` | undefined;

if (existingEntryPoint) {
  entryPointAddress = normalizeAddress(existingEntryPoint, "HORIZEN_AA_ENTRYPOINT_ADDRESS");
  const code = await publicClient.getCode({ address: entryPointAddress });
  if (!code || code === "0x") {
    throw new Error(`HORIZEN_AA_ENTRYPOINT_ADDRESS has no code: ${entryPointAddress}`);
  }
  console.log(`Using existing EntryPoint ${entryPointAddress}`);
} else {
  const artifact = loadArtifact("EntryPoint");
  entryPointHash = await walletClient.deployContract({
    abi: artifact.abi,
    bytecode: artifact.bytecode,
  });
  console.log(`EntryPoint tx: ${entryPointHash}`);
  const receipt = await publicClient.waitForTransactionReceipt({ hash: entryPointHash });
  if (!receipt.contractAddress) {
    throw new Error("EntryPoint deployment did not return a contract address");
  }
  entryPointAddress = getAddress(receipt.contractAddress) as Address;
  console.log(`EntryPoint deployed: ${entryPointAddress}`);
}

const factoryArtifact = loadArtifact("SimpleAccountFactory");
const factoryHash = await walletClient.deployContract({
  abi: factoryArtifact.abi,
  args: [entryPointAddress],
  bytecode: factoryArtifact.bytecode,
});
console.log(`SimpleAccountFactory tx: ${factoryHash}`);
const factoryReceipt = await publicClient.waitForTransactionReceipt({ hash: factoryHash });
if (!factoryReceipt.contractAddress) {
  throw new Error("SimpleAccountFactory deployment did not return a contract address");
}

const deployment: AADeployment = {
  accountAbstractionContractsPackage: "@account-abstraction/contracts@0.8.0",
  bundlerUrl: bundlerUrl(),
  chainId: horizenTestnet.id,
  chainName: horizenTestnet.name,
  deployedAt: new Date().toISOString(),
  deployer,
  entryPoint: {
    address: entryPointAddress,
    transactionHash: entryPointHash,
    version: "0.8",
  },
  explorerUrl: horizenTestnet.blockExplorers.default.url,
  factory: {
    address: getAddress(factoryReceipt.contractAddress) as Address,
    transactionHash: factoryHash,
    type: "SimpleAccountFactory",
  },
  rpcUrl: rpcUrl(),
};

saveDeployment(deployment);

console.log(`SimpleAccountFactory deployed: ${deployment.factory.address}`);
console.log(`Saved ${deploymentPath}`);
console.log("\nAPI/frontend env:");
console.log(deploymentEnv(deployment));
