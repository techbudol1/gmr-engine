import { createPublicClient, createWalletClient, defineChain, http, parseAbi } from "viem";
import { privateKeyToAccount } from "viem/accounts";

type MintRequest = {
  amount: string;
  chainId: number;
  contractAddress: `0x${string}`;
  privateKey: `0x${string}`;
  recipient: `0x${string}`;
  rpcUrl: string;
};

const body = await new Response(Bun.stdin.stream()).text();
const request = JSON.parse(body) as MintRequest;
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
const abi = parseAbi(["function mint(address to, uint256 value) external"]);

const transactionHash = await walletClient.writeContract({
  abi,
  account,
  address: request.contractAddress,
  args: [request.recipient, BigInt(request.amount)],
  functionName: "mint",
});
const receipt = await publicClient.waitForTransactionReceipt({ hash: transactionHash });

console.log(JSON.stringify({
  status: receipt.status,
  transactionHash,
}));
