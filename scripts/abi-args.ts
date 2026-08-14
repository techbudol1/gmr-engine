export type ABIInput = {
  components?: ABIInput[];
  name?: string;
  type: string;
};

export function coerceABIArg(value: unknown, input: ABIInput): unknown {
  if (input.type.endsWith("[]")) {
    const values = parseCollection(value);
    const itemInput = { ...input, type: input.type.slice(0, -2) };
    return values.map(item => coerceABIArg(item, itemInput));
  }

  if (input.type === "tuple") {
    const tuple = parseJSONValue(value);
    const components = input.components ?? [];
    if (Array.isArray(tuple)) {
      return components.map((component, index) => coerceABIArg(tuple[index], component));
    }
    if (tuple && typeof tuple === "object") {
      const record = tuple as Record<string, unknown>;
      return Object.fromEntries(components.map(component => [
        component.name ?? "",
        coerceABIArg(record[component.name ?? ""], component),
      ]));
    }
    throw new Error("tuple argument must be a JSON object or array");
  }

  const text = value == null ? "" : String(value).trim();
  if (input.type.startsWith("uint") || input.type.startsWith("int")) return BigInt(text || "0");
  if (input.type === "bool") return ["1", "true", "yes", "on"].includes(text.toLowerCase());
  if (input.type === "bytes" || /^bytes\d+$/.test(input.type)) return text || "0x";
  return text;
}

function parseCollection(value: unknown): unknown[] {
  if (Array.isArray(value)) return value;
  const text = value == null ? "" : String(value).trim();
  if (!text) return [];
  if (text.startsWith("[")) {
    const parsed = JSON.parse(text);
    if (!Array.isArray(parsed)) throw new Error("array argument must be a JSON array");
    return parsed;
  }
  return text.split(",").map(item => item.trim()).filter(Boolean);
}

function parseJSONValue(value: unknown): unknown {
  if (typeof value !== "string") return value;
  const text = value.trim();
  if (!text) throw new Error("tuple argument is required");
  return JSON.parse(text);
}
