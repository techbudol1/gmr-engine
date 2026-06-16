import { createPublicClient, createWalletClient, defineChain, formatUnits, http, parseAbi } from "viem";
import { privateKeyToAccount } from "viem/accounts";

type TokenAction = "mint" | "transfer" | "burn" | "airdrop";

type Recipient = {
  address: `0x${string}`;
  amount: string;
};

type Request = {
  action?: TokenAction;
  amount?: string;
  chainId: number;
  contractAddress: `0x${string}`;
  mode: "read" | "write";
  privateKey?: `0x${string}`;
  recipient?: `0x${string}`;
  recipients?: Recipient[];
  rpcUrl: string;
  walletAddress: `0x${string}`;
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
const abi = parseAbi([
  "function name() view returns (string)",
  "function symbol() view returns (string)",
  "function decimals() view returns (uint8)",
  "function totalSupply() view returns (uint256)",
  "function balanceOf(address account) view returns (uint256)",
  "function mint(address to, uint256 value) external",
  "function transfer(address to, uint256 value) external returns (bool)",
  "function burn(uint256 value) external",
]);

if (request.mode === "read") {
  const [name, symbol, decimals, totalSupply, ownedBalance] = await Promise.all([
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "name" }),
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "symbol" }),
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "decimals" }),
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "totalSupply" }),
    publicClient.readContract({ address: request.contractAddress, abi, functionName: "balanceOf", args: [request.walletAddress] }),
  ]);
  console.log(JSON.stringify({
    decimals,
    name,
    ownedBalance: formatUnits(ownedBalance, decimals),
    ownedBalanceRaw: ownedBalance.toString(),
    symbol,
    totalSupply: formatUnits(totalSupply, decimals),
    totalSupplyRaw: totalSupply.toString(),
    walletAddress: request.walletAddress,
  }));
  process.exit(0);
}

if (!request.privateKey) throw new Error("private key is required");
const account = privateKeyToAccount(request.privateKey);
const walletClient = createWalletClient({ account, chain, transport });

async function wait(hash: `0x${string}`) {
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  return { status: receipt.status, transactionHash: hash };
}

const transactions = [];
if (request.action === "mint") {
  if (!request.recipient || !request.amount) throw new Error("recipient and amount are required");
  const hash = await walletClient.writeContract({
    abi,
    account,
    address: request.contractAddress,
    args: [request.recipient, BigInt(request.amount)],
    functionName: "mint",
  });
  transactions.push(await wait(hash));
} else if (request.action === "transfer") {
  if (!request.recipient || !request.amount) throw new Error("recipient and amount are required");
  const hash = await walletClient.writeContract({
    abi,
    account,
    address: request.contractAddress,
    args: [request.recipient, BigInt(request.amount)],
    functionName: "transfer",
  });
  transactions.push(await wait(hash));
} else if (request.action === "burn") {
  if (!request.amount) throw new Error("amount is required");
  const hash = await walletClient.writeContract({
    abi,
    account,
    address: request.contractAddress,
    args: [BigInt(request.amount)],
    functionName: "burn",
  });
  transactions.push(await wait(hash));
} else if (request.action === "airdrop") {
  if (!request.recipients?.length) throw new Error("recipients are required");
  for (const recipient of request.recipients) {
    const hash = await walletClient.writeContract({
      abi,
      account,
      address: request.contractAddress,
      args: [recipient.address, BigInt(recipient.amount)],
      functionName: "transfer",
    });
    transactions.push(await wait(hash));
  }
} else {
  throw new Error("unsupported token action");
}

console.log(JSON.stringify({ transactions }));
