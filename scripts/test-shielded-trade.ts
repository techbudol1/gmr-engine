import { buildPoseidon } from "circomlibjs";
import { readFile } from "node:fs/promises";
import { groth16 } from "snarkjs";

const rootDir = process.cwd();
const buildDir = `${rootDir}/zk/shielded-trade/build`;
const wasm = `${buildDir}/shielded_trade_js/shielded_trade.wasm`;
const zkey = `${buildDir}/shielded_trade_final.zkey`;
const verificationKey = JSON.parse(await readFile(`${buildDir}/verification_key.json`, "utf8"));

const poseidon = await buildPoseidon();
const field = poseidon.F;
const hash = (values: bigint[]) => BigInt(field.toString(poseidon(values)));
const modulus = 21888242871839275222246405745257275088548364400416034343698204186575808495617n;
const inverse = (value: bigint) => {
  let a = value % modulus;
  let b = modulus;
  let x = 1n;
  let y = 0n;
  while (b !== 0n) {
    const q = a / b;
    [a, b] = [b, a - q * b];
    [x, y] = [y, x - q * y];
  }
  return ((x % modulus) + modulus) % modulus;
};

const chainId = 2651420n;
const tokenAddress = BigInt("0x689513fb392e460c6d9225f911fce57fe50d6db4");
const vaultAddress = BigInt("0x1111111111111111111111111111111111111111");
const denomination = 10050000000000000000n;
const feeBps = 50n;
const batchId = 123456789n;
const secret = 111n;
const blinding = 222n;
const marketId = 333n;
const outcome = 1n;
const tradeAmount = 10000000000000000000n;
const orderSalt = 444n;

const leaf = hash([secret, blinding, chainId, tokenAddress, vaultAddress, denomination]);
const pathElements: bigint[] = [];
const pathIndices = Array<bigint>(20).fill(0n);
let zero = 0n;
let root = leaf;
for (let level = 0; level < 20; level += 1) {
  pathElements.push(zero);
  root = hash([root, zero]);
  zero = hash([zero, zero]);
}

const nullifierHash = hash([secret, blinding, vaultAddress]);
const orderCommitment = hash([marketId, outcome, tradeAmount, orderSalt, batchId]);
const input = {
  root: root.toString(),
  nullifierHash: nullifierHash.toString(),
  orderCommitment: orderCommitment.toString(),
  chainId: chainId.toString(),
  tokenAddress: tokenAddress.toString(),
  vaultAddress: vaultAddress.toString(),
  denomination: denomination.toString(),
  feeBps: feeBps.toString(),
  batchId: batchId.toString(),
  secret: secret.toString(),
  blinding: blinding.toString(),
  marketId: marketId.toString(),
  outcome: outcome.toString(),
  tradeAmount: tradeAmount.toString(),
  tradeAmountInverse: inverse(tradeAmount).toString(),
  orderSalt: orderSalt.toString(),
  pathElements: pathElements.map(String),
  pathIndices: pathIndices.map(String),
};

const { proof, publicSignals } = await groth16.fullProve(input, wasm, zkey);
if (!await groth16.verify(verificationKey, publicSignals, proof)) {
  throw new Error("shielded trade proof did not verify");
}
if (publicSignals.map(String).join(",") !== [
  root,
  nullifierHash,
  orderCommitment,
  chainId,
  tokenAddress,
  vaultAddress,
  denomination,
  feeBps,
  batchId,
].map(String).join(",")) {
  throw new Error("shielded trade public signal order changed");
}

console.log(JSON.stringify({
  ok: true,
  orderCommitment: orderCommitment.toString(),
  publicSignalCount: publicSignals.length,
  root: root.toString(),
}));
