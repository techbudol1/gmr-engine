import { describe, expect, test } from "bun:test";
import { coerceABIArg, type ABIInput } from "./abi-args";

const listingInput: ABIInput = {
  name: "params",
  type: "tuple",
  components: [
    { name: "assetContract", type: "address" },
    { name: "tokenId", type: "uint256" },
    { name: "quantity", type: "uint256" },
    { name: "reserved", type: "bool" },
  ],
};

describe("coerceABIArg", () => {
  test("coerces named tuple fields for marketplace listing calls", () => {
    expect(coerceABIArg(JSON.stringify({
      assetContract: "0x1234",
      tokenId: "7",
      quantity: "2",
      reserved: false,
    }), listingInput)).toEqual({
      assetContract: "0x1234",
      tokenId: 7n,
      quantity: 2n,
      reserved: false,
    });
  });

  test("coerces tuple arrays recursively", () => {
    expect(coerceABIArg('[{"assetContract":"0xabcd","tokenId":"1","quantity":"1","reserved":true}]', {
      ...listingInput,
      type: "tuple[]",
    })).toEqual([{
      assetContract: "0xabcd",
      tokenId: 1n,
      quantity: 1n,
      reserved: true,
    }]);
  });
});
