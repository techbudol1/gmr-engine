import { createPublicClient, createWalletClient, defineChain, http } from "viem";
import { privateKeyToAccount } from "viem/accounts";

type ConstructorArg = {
  kind: "address" | "string" | "uint" | "uint8";
  value: string;
};

type DeployRequest = {
  abi: string;
  args: ConstructorArg[];
  bytecode: string;
  chainId: number;
  privateKey: `0x${string}`;
  rpcUrl: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as DeployRequest;
const chain = defineChain({
  id: request.chainId,
  name: `Chain ${request.chainId}`,
  nativeCurrency: { decimals: 18, name: "Ether", symbol: "ETH" },
  rpcUrls: {
    default: { http: [request.rpcUrl] },
  },
});

const account = privateKeyToAccount(request.privateKey);
const transport = http(request.rpcUrl);
const walletClient = createWalletClient({ account, chain, transport });
const publicClient = createPublicClient({ chain, transport });
const args = request.args.map(arg => {
  if (arg.kind === "uint") return BigInt(arg.value);
  if (arg.kind === "uint8") return Number(arg.value);
  return arg.value;
});
const bytecode = request.bytecode.startsWith("0x") ? request.bytecode as `0x${string}` : `0x${request.bytecode}` as `0x${string}`;
const transactionHash = await walletClient.deployContract({
  abi: JSON.parse(request.abi),
  args,
  bytecode,
});
const receipt = await publicClient.waitForTransactionReceipt({ hash: transactionHash });

console.log(JSON.stringify({
  contractAddress: receipt.contractAddress,
  status: receipt.status,
  transactionHash,
}));
