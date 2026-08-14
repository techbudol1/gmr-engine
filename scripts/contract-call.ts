import { createPublicClient, createWalletClient, defineChain, http } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount, hasVaultSigner } from "./vault-account";
import { coerceABIArg, type ABIInput } from "./abi-args";

type ABIFunction = {
  inputs?: ABIInput[];
  name: string;
  stateMutability?: string;
  type: string;
};

type Request = {
  abi: ABIFunction[];
  args?: string[];
  chainId: number;
  contractAddress: `0x${string}`;
  functionName: string;
  mode: "read" | "write";
  privateKey?: `0x${string}`;
  rpcUrl: string;
  value?: string;
  vaultAddress?: `0x${string}`;
  vaultApiKey?: string;
  vaultProjectId?: string;
  vaultUrl?: string;
  vaultWalletRef?: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as Request;
const chain = defineChain({
  id: request.chainId,
  name: `Chain ${request.chainId}`,
  nativeCurrency: { decimals: 18, name: "Ether", symbol: "ETH" },
  rpcUrls: {
    default: { http: [request.rpcUrl] },
  },
});
const transport = http(request.rpcUrl);
const publicClient = createPublicClient({ chain, transport });
const fn = request.abi.find(item => item.type === "function" && item.name === request.functionName);
if (!fn) throw new Error(`function not found in ABI: ${request.functionName}`);
const args = (fn.inputs ?? []).map((input, index) => coerceABIArg(request.args?.[index] ?? "", input));

function stringify(value: unknown) {
  return JSON.stringify(value, (_key, item) => typeof item === "bigint" ? item.toString() : item);
}

if (request.mode === "read") {
  const result = await publicClient.readContract({
    abi: request.abi as any,
    address: request.contractAddress,
    args,
    functionName: request.functionName,
  });
  console.log(stringify({ result }));
  process.exit(0);
}

if (!request.privateKey && !hasVaultSigner(request)) throw new Error("private key or vault signer is required");
const account = hasVaultSigner(request) ? createVaultAccount(request) : privateKeyToAccount(request.privateKey!);
const walletClient = createWalletClient({ account, chain, transport });
const hash = await walletClient.writeContract({
  abi: request.abi as any,
  account,
  address: request.contractAddress,
  args,
  functionName: request.functionName,
  value: request.value ? BigInt(request.value) : 0n,
});
const receipt = await publicClient.waitForTransactionReceipt({ hash });
console.log(stringify({ transactions: [{ status: receipt.status, transactionHash: hash }] }));
