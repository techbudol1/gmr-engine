import { createPublicClient, createWalletClient, defineChain, getAddress, http, type Address, type Hex } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount, hasVaultSigner } from "../vault-account";
import { bundlerUrl, horizenTestnet, loadArtifact, normalizeAddress } from "./horizen-aa";

type DeployAARequest = {
  chainId: number;
  entryPointAddress?: string;
  privateKey?: Hex;
  rpcUrl: string;
  vaultAddress?: Address;
  vaultApiKey?: string;
  vaultProjectId?: string;
  vaultUrl?: string;
  vaultWalletRef?: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as DeployAARequest;

if (request.chainId !== horizenTestnet.id) {
  throw new Error(`AA dashboard deployment currently supports Horizen Testnet only; got chain ${request.chainId}`);
}
if (!request.rpcUrl) {
  throw new Error("rpcUrl is required");
}

const chain = defineChain({
  id: request.chainId,
  name: horizenTestnet.name,
  nativeCurrency: horizenTestnet.nativeCurrency,
  rpcUrls: {
    default: { http: [request.rpcUrl] },
  },
  blockExplorers: horizenTestnet.blockExplorers,
});
const account = hasVaultSigner(request) ? createVaultAccount(request) : privateKeyToAccount(request.privateKey);
const transport = http(request.rpcUrl);
const publicClient = createPublicClient({ chain, transport });
const walletClient = createWalletClient({ account, chain, transport });
const actualChainId = await publicClient.getChainId();

if (actualChainId !== horizenTestnet.id) {
  throw new Error(`RPC returned chain ${actualChainId}; expected Horizen Testnet ${horizenTestnet.id}`);
}

let entryPointAddress: Address;
let entryPointTransactionHash = "";
const existingEntryPoint = request.entryPointAddress?.trim();

if (existingEntryPoint) {
  entryPointAddress = normalizeAddress(existingEntryPoint, "entryPointAddress");
  const code = await publicClient.getCode({ address: entryPointAddress });
  if (!code || code === "0x") {
    throw new Error(`entryPointAddress has no code: ${entryPointAddress}`);
  }
} else {
  const entryPoint = loadArtifact("EntryPoint");
  const txHash = await walletClient.deployContract({
    abi: entryPoint.abi,
    bytecode: entryPoint.bytecode,
  });
  const receipt = await publicClient.waitForTransactionReceipt({ hash: txHash });
  if (receipt.status !== "success" || !receipt.contractAddress) {
    throw new Error("EntryPoint deployment failed");
  }
  entryPointAddress = getAddress(receipt.contractAddress) as Address;
  entryPointTransactionHash = txHash;
}

const factory = loadArtifact("SimpleAccountFactory");
const factoryTransactionHash = await walletClient.deployContract({
  abi: factory.abi,
  args: [entryPointAddress],
  bytecode: factory.bytecode,
});
const factoryReceipt = await publicClient.waitForTransactionReceipt({ hash: factoryTransactionHash });
if (factoryReceipt.status !== "success" || !factoryReceipt.contractAddress) {
  throw new Error("SimpleAccountFactory deployment failed");
}

console.log(JSON.stringify({
  bundlerUrl: bundlerUrl(),
  contractAddress: getAddress(factoryReceipt.contractAddress),
  entryPointAddress,
  entryPointTransactionHash,
  factoryAddress: getAddress(factoryReceipt.contractAddress),
  factoryTransactionHash,
  status: factoryReceipt.status,
  transactionHash: factoryTransactionHash,
  version: "0.8",
}));
