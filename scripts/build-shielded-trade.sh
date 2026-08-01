#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CIRCUIT_DIR="$ROOT/zk/shielded-trade"
BUILD_DIR="$CIRCUIT_DIR/build"
PTAU="${PTAU_FILE:-$BUILD_DIR/pot16_final.ptau}"
SNARKJS="$ROOT/node_modules/.bin/snarkjs"

mkdir -p "$BUILD_DIR"
circom "$CIRCUIT_DIR/shielded_trade.circom" \
  --r1cs --wasm --sym \
  -l "$ROOT/node_modules" \
  -o "$BUILD_DIR"

if [[ ! -f "$PTAU" ]]; then
  "$SNARKJS" powersoftau new bn128 16 "$BUILD_DIR/pot16_0000.ptau" -v
  "$SNARKJS" powersoftau contribute "$BUILD_DIR/pot16_0000.ptau" "$BUILD_DIR/pot16_0001.ptau" \
    --name="BudolPH shielded trade development contribution" -e="budolph-shielded-trade-testnet"
  "$SNARKJS" powersoftau prepare phase2 "$BUILD_DIR/pot16_0001.ptau" "$PTAU" -v
fi

"$SNARKJS" groth16 setup "$BUILD_DIR/shielded_trade.r1cs" "$PTAU" "$BUILD_DIR/shielded_trade_0000.zkey"
"$SNARKJS" zkey contribute "$BUILD_DIR/shielded_trade_0000.zkey" "$BUILD_DIR/shielded_trade_final.zkey" \
  --name="BudolPH shielded trade development phase 2" -e="budolph-shielded-trade-phase2-testnet"
"$SNARKJS" zkey export verificationkey "$BUILD_DIR/shielded_trade_final.zkey" "$BUILD_DIR/verification_key.json"
"$SNARKJS" zkey export solidityverifier "$BUILD_DIR/shielded_trade_final.zkey" "$BUILD_DIR/ShieldedTradeVerifier.sol"

echo "Shielded trade development artifacts written to $BUILD_DIR"
