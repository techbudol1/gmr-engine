import { readFile } from "node:fs/promises";
import path from "node:path";
import { poseidonContract } from "circomlibjs";
import solc from "solc";
import {
  createPublicClient,
  createWalletClient,
  defineChain,
  getAddress,
  http,
  type Abi,
  type Address,
  type Hex,
} from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount, hasVaultSigner } from "../vault-account";

type DeployRequest = {
  chainId: number;
  denomination: string;
  escrowAddress: Address;
  feeBps: string | number;
  ownerAddress?: Address;
  privateKey?: Hex;
  rpcUrl: string;
  tokenAddress: Address;
  vaultAddress?: Address;
  vaultApiKey?: string;
  vaultProjectId?: string;
  vaultUrl?: string;
  vaultWalletRef?: string;
};

type Artifact = { abi: Abi; bytecode: Hex };

function compile(sourcePath: string, source: string, contractName: string): Artifact {
  const input = {
    language: "Solidity",
    sources: { [sourcePath]: { content: source } },
    settings: {
      optimizer: { enabled: true, runs: 200 },
      outputSelection: { "*": { "*": ["abi", "evm.bytecode.object"] } },
    },
  };
  const output = JSON.parse(solc.compile(JSON.stringify(input)));
  const errors = (output.errors ?? []).filter((item: { severity: string }) => item.severity === "error");
  if (errors.length > 0) {
    throw new Error(errors.map((item: { formattedMessage: string }) => item.formattedMessage).join("\n"));
  }
  const artifact = output.contracts?.[sourcePath]?.[contractName];
  if (!artifact?.evm?.bytecode?.object) throw new Error(`compiled artifact missing: ${contractName}`);
  return {
    abi: artifact.abi as Abi,
    bytecode: `0x${artifact.evm.bytecode.object}` as Hex,
  };
}

const raw = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(raw) as DeployRequest;
if (request.chainId !== 2651420) throw new Error("shielded trade deployment currently supports Horizen Testnet (2651420) only");
if (!request.rpcUrl) throw new Error("rpcUrl is required");
if (!/^0x[0-9a-fA-F]{40}$/.test(request.tokenAddress)) throw new Error("tokenAddress is invalid");
if (!/^0x[0-9a-fA-F]{40}$/.test(request.escrowAddress)) throw new Error("escrowAddress is invalid");
if (!/^\d+$/.test(request.denomination) || BigInt(request.denomination) <= 0n) throw new Error("denomination must be positive base units");
if (!hasVaultSigner(request) && !/^0x[0-9a-fA-F]{64}$/.test(request.privateKey ?? "")) throw new Error("Vault signer configuration or privateKey is required");
const feeBps = BigInt(request.feeBps);
if (feeBps < 0n || feeBps > 1_000n) throw new Error("feeBps must be between 0 and 1000");

const chain = defineChain({
  id: request.chainId,
  name: "Horizen Testnet",
  nativeCurrency: { decimals: 18, name: "Ether", symbol: "ETH" },
  rpcUrls: { default: { http: [request.rpcUrl] } },
  blockExplorers: { default: { name: "Horizen Explorer", url: "https://horizen-testnet.explorer.caldera.xyz" } },
});
const account = hasVaultSigner(request) ? createVaultAccount(request) : privateKeyToAccount(request.privateKey);
const ownerAddress = getAddress(request.ownerAddress ?? account.address);
const transport = http(request.rpcUrl);
const publicClient = createPublicClient({ chain, transport });
const walletClient = createWalletClient({ account, chain, transport });
if (await publicClient.getChainId() !== request.chainId) throw new Error("RPC chain ID mismatch");

const deploy = async (artifact: Artifact, args: readonly unknown[] = []) => {
  const hash = await walletClient.deployContract({
    abi: artifact.abi,
    account,
    args,
    bytecode: artifact.bytecode,
  });
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success" || !receipt.contractAddress) throw new Error(`deployment failed: ${hash}`);
  return { address: getAddress(receipt.contractAddress), hash };
};

const poseidonArtifact: Artifact = {
  abi: poseidonContract.generateABI(2) as Abi,
  bytecode: poseidonContract.createCode(2) as Hex,
};
const poseidon = await deploy(poseidonArtifact);

const verifierPath = path.resolve("zk/shielded-trade/build/ShieldedTradeVerifier.sol");
const verifierSource = await readFile(verifierPath, "utf8");
const verifierArtifact = compile("ShieldedTradeVerifier.sol", verifierSource, "Groth16Verifier");
const verifier = await deploy(verifierArtifact);

const vaultPath = path.resolve("contracts/privacy/BudolShieldedTradeVault.sol");
const vaultSource = await readFile(vaultPath, "utf8");
const vaultArtifact = compile("BudolShieldedTradeVault.sol", vaultSource, "BudolShieldedTradeVault");
const vault = await deploy(vaultArtifact, [
  getAddress(request.tokenAddress),
  poseidon.address,
  verifier.address,
  getAddress(request.escrowAddress),
  ownerAddress,
  BigInt(request.denomination),
  feeBps,
]);

console.log(JSON.stringify({
  chainId: request.chainId,
  denomination: request.denomination,
  escrowAddress: getAddress(request.escrowAddress),
  feeBps: feeBps.toString(),
  ownerAddress,
  poseidonAddress: poseidon.address,
  poseidonTransactionHash: poseidon.hash,
  tokenAddress: getAddress(request.tokenAddress),
  vaultAddress: vault.address,
  vaultTransactionHash: vault.hash,
  verifierAddress: verifier.address,
  verifierTransactionHash: verifier.hash,
}));
