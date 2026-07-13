import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import solc from "solc";
import { createPublicClient, createWalletClient, getAddress, http, parseEther, type Address, type Hex } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import { createVaultAccount } from "../vault-account";
import { horizenTestnet, normalizeAddress, rpcUrl } from "./horizen-aa";

const sourcePath = resolve("contracts/aa/BudolTradePaymaster.sol");
const source = readFileSync(sourcePath, "utf8");

const entryPointAddress = normalizeAddress(env("HORIZEN_AA_ENTRYPOINT_ADDRESS"), "HORIZEN_AA_ENTRYPOINT_ADDRESS");
const tokenAddress = normalizeAddress(env("BUDOL_TOKEN_ADDRESS", env("GMR_ENGINE_BUDOL_TOKEN_ADDRESS")), "BUDOL_TOKEN_ADDRESS");
const escrowAddress = normalizeAddress(env("BUDOL_ESCROW_WALLET_ADDRESS", env("GMR_ENGINE_ESCROW_WALLET_ADDRESS", env("HORIZEN_BUNDLER_ADDRESS"))), "BUDOL_ESCROW_WALLET_ADDRESS");
const signerAddress = normalizeAddress(env("HORIZEN_PAYMASTER_SIGNER_ADDRESS", env("HORIZEN_BUNDLER_ADDRESS")), "HORIZEN_PAYMASTER_SIGNER_ADDRESS");
const depositAmount = parseEther(env("HORIZEN_PAYMASTER_INITIAL_DEPOSIT_ETH", "0.005"));

const artifact = compilePaymaster();
const account = deployerAccount();
const publicClient = createPublicClient({ chain: horizenTestnet, transport: http(rpcUrl()) });
const walletClient = createWalletClient({ account, chain: horizenTestnet, transport: http(rpcUrl()) });

const chainId = await publicClient.getChainId();
if (chainId !== horizenTestnet.id) {
  throw new Error(`RPC returned chain ${chainId}; expected ${horizenTestnet.id}`);
}

console.log(JSON.stringify({
  deployer: getAddress(account.address),
  depositAmountWei: depositAmount.toString(),
  entryPoint: entryPointAddress,
  escrow: escrowAddress,
  rpcUrl: rpcUrl(),
  signer: signerAddress,
  token: tokenAddress,
}, null, 2));

const deployHash = await walletClient.deployContract({
  abi: artifact.abi,
  args: [entryPointAddress, signerAddress, tokenAddress, escrowAddress],
  bytecode: artifact.bytecode,
});
console.log(`BudolTradePaymaster deploy tx: ${deployHash}`);
const deployReceipt = await publicClient.waitForTransactionReceipt({ hash: deployHash });
if (!deployReceipt.contractAddress) {
  throw new Error("Paymaster deployment did not return a contract address");
}
const paymasterAddress = getAddress(deployReceipt.contractAddress) as Address;
console.log(`BudolTradePaymaster deployed: ${paymasterAddress}`);

if (depositAmount > 0n) {
  const depositHash = await walletClient.writeContract({
    abi: artifact.abi,
    address: paymasterAddress,
    args: [],
    functionName: "addDeposit",
    value: depositAmount,
  });
  console.log(`Paymaster EntryPoint deposit tx: ${depositHash}`);
  await publicClient.waitForTransactionReceipt({ hash: depositHash });
}

const deposit = await publicClient.readContract({
  abi: artifact.abi,
  address: paymasterAddress,
  args: [],
  functionName: "getDeposit",
}) as bigint;

const output = {
  chainId: horizenTestnet.id,
  deployedAt: new Date().toISOString(),
  deployer: getAddress(account.address),
  entryPoint: entryPointAddress,
  escrow: escrowAddress,
  paymaster: paymasterAddress,
  paymasterDepositWei: deposit.toString(),
  signer: signerAddress,
  token: tokenAddress,
  transactionHash: deployHash,
};
mkdirSync(resolve("deployments"), { recursive: true });
writeFileSync(resolve("deployments/horizen-trade-paymaster.json"), `${JSON.stringify(output, null, 2)}\n`);

console.log("\nAPI/frontend env:");
console.log(`HORIZEN_AA_PAYMASTER_ADDRESS=${paymasterAddress}`);
console.log(`HORIZEN_AA_PAYMASTER_URL=${env("HORIZEN_AA_BUNDLER_URL", "https://bundler.budolph.xyz")}`);
console.log(`HORIZEN_AA_GAS_SPONSORED=true`);

function compilePaymaster() {
  const input = {
    language: "Solidity",
    sources: {
      "BudolTradePaymaster.sol": {
        content: source,
      },
    },
    settings: {
      optimizer: {
        enabled: true,
        runs: 200,
      },
      outputSelection: {
        "*": {
          "*": ["abi", "evm.bytecode.object"],
        },
      },
    },
  };
  const output = JSON.parse(solc.compile(JSON.stringify(input)));
  const errors = (output.errors || []).filter((item: { severity?: string }) => item.severity === "error");
  if (errors.length) {
    throw new Error(errors.map((item: { formattedMessage?: string }) => item.formattedMessage || String(item)).join("\n"));
  }
  const contract = output.contracts?.["BudolTradePaymaster.sol"]?.BudolTradePaymaster;
  if (!contract?.abi || !contract?.evm?.bytecode?.object) {
    throw new Error("BudolTradePaymaster compile output is missing ABI or bytecode");
  }
  return {
    abi: contract.abi,
    bytecode: `0x${contract.evm.bytecode.object}` as Hex,
  };
}

function deployerAccount() {
  const vaultWalletRef = env("HORIZEN_BUNDLER_VAULT_WALLET_REF");
  if (vaultWalletRef) {
    return createVaultAccount({
      chainId: horizenTestnet.id,
      rpcUrl: rpcUrl(),
      vaultAddress: normalizeAddress(env("HORIZEN_BUNDLER_ADDRESS"), "HORIZEN_BUNDLER_ADDRESS"),
      vaultApiKey: env("HORIZEN_BUNDLER_VAULT_API_KEY", env("GMR_ENGINE_VAULT_INTERNAL_API_KEY")),
      vaultUrl: env("HORIZEN_BUNDLER_VAULT_URL", env("GMR_ENGINE_VAULT_URL")),
      vaultWalletRef,
    });
  }
  const privateKey = env("HORIZEN_PAYMASTER_DEPLOYER_PRIVATE_KEY", env("HORIZEN_BUNDLER_PRIVATE_KEY"));
  if (!/^0x[0-9a-fA-F]{64}$/.test(privateKey)) {
    throw new Error("HORIZEN_PAYMASTER_DEPLOYER_PRIVATE_KEY or HORIZEN_BUNDLER_PRIVATE_KEY must be set");
  }
  return privateKeyToAccount(privateKey as Hex);
}

function env(key: string, fallback = "") {
  return process.env[key]?.trim() || fallback;
}
