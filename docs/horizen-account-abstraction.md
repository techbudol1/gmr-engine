# BudolPH Horizen Account Abstraction

This module deploys and configures ERC-4337 smart accounts for BudolPH on Horizen testnet.

## Standards

- ERC-4337 account abstraction
- Eth Infinitism `@account-abstraction/contracts@0.8.0`
- `EntryPoint` version: `0.8`
- Account implementation: `SimpleAccount`
- Factory: `SimpleAccountFactory`
- Bundler client compatibility: `permissionless` / ERC-4337 JSON-RPC bundlers

Primary sources:

- https://github.com/eth-infinitism/account-abstraction
- https://docs.erc4337.io/
- https://docs.horizen.io/overview/rpc/

## Deployment

Set a funded Horizen testnet deployer key:

```bash
export HORIZEN_AA_DEPLOYER_PRIVATE_KEY=0x...
export HORIZEN_AA_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
```

Deploy EntryPoint and SimpleAccountFactory:

```bash
bun run aa:deploy:horizen
```

If a valid EntryPoint already exists on Horizen testnet, reuse it:

```bash
export HORIZEN_AA_ENTRYPOINT_ADDRESS=0x...
bun run aa:deploy:horizen
```

The script writes:

```text
deployments/horizen-aa-testnet.json
```

## API/frontend env

Print the env values after deployment:

```bash
bun run aa:env:horizen
```

Expected keys:

```env
HORIZEN_AA_ENABLED=true
HORIZEN_AA_CHAIN_ID=2651420
HORIZEN_AA_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
HORIZEN_AA_ENTRYPOINT_ADDRESS=0x...
HORIZEN_AA_ENTRYPOINT_VERSION=0.8
HORIZEN_AA_FACTORY_ADDRESS=0x...
HORIZEN_AA_BUNDLER_URL=<set-after-bundler-is-running>
```

## Derive a user smart wallet address

```bash
HORIZEN_AA_OWNER_ADDRESS=0x... bun run aa:derive:horizen
```

or:

```bash
bun run aa:derive:horizen -- 0xOwnerAddress
```

The derived account is counterfactual until the first UserOperation deploys it through the EntryPoint.

## Bundler requirement

This does not assume Alchemy or thirdweb chain support. Run a self-hosted ERC-4337 bundler pointed at:

```text
https://horizen-testnet.rpc.caldera.xyz/http
```

The bundler must be configured for:

- chain ID `2651420`
- EntryPoint `0.8`
- the deployed EntryPoint address from `deployments/horizen-aa-testnet.json`

## Smoke-test a UserOperation

After the bundler is running and the counterfactual smart account has ETH for gas:

```bash
export HORIZEN_AA_OWNER_PRIVATE_KEY=0x...
export HORIZEN_AA_BUNDLER_URL=https://...
bun run aa:smoke:horizen
```

This submits a no-op UserOperation from the SimpleAccount to itself. On first use, it also deploys the account through the factory.

## MVP sequence

1. Deploy EntryPoint and SimpleAccountFactory.
2. Run an ERC-4337 bundler against Horizen testnet.
3. Fund a counterfactual smart account with ETH for gas.
4. Submit a no-op or BUDOL approval UserOperation.
5. Convert BudolPH trade escrow to UserOperation execution.
6. Add paymaster only after basic smart-wallet execution is stable.
