export const SType = {
  Error: 0,
  End: 1,
  Object: 2,
  Array: 3,
  Float: 4,
  Bool: 5,
  String: 6,
} as const;
export type SType = typeof SType[keyof typeof SType];

export const EType = {
  Record: 0x01,

  SYN: 0x10,
  ACK: 0x11,
  
  Typing: 0x20,
  PresenceUpdate: 0x21,
  Auth: 0x90,
  
  Error: 0xFE,
  Reserved: 0xFF,
} as const;
export type EType = typeof EType[keyof typeof EType];

export const RType = {
  Message: 0,
  DeleteMessage: 1,
  Edit: 2,
  Reply: 3,
  ReactionAdd: 4,
  ReactionRemove: 5,
} as const;
export type RType = typeof RType[keyof typeof RType];

export type ChatRecord = {
  type: RType,
  sn?: number,
  text: string,
};

export type RecordEvent = {
  type: typeof EType.Record,
  record: ChatRecord,
};

export type SYNEvent = {
  type: typeof EType.SYN,
  sn: number,
};

export type ACKEvent = {
  type: typeof EType.ACK,
  sn: number,
};

export type ChatEvent = RecordEvent | SYNEvent | ACKEvent;

export function deserialize(msg: Uint8Array, ctx = { cur: 0 }): any {
  const dv = new DataView(msg.buffer);

  const tag = dv.getUint8(ctx.cur);
  ctx.cur += 1;

  switch (tag) {
    case SType.Object: {
      const result: Record<string, any> = {};
      while (true) {
        const fieldTag = dv.getUint8(ctx.cur);
        if (fieldTag === SType.End) {
          ctx.cur += 1;
          break
        }
        if (fieldTag !== SType.String) {
          throw new Error(`expected string for field name (type ${SType.String}), but got type ${fieldTag}`);
        }
        const fieldName = deserialize(msg, ctx);
        const fieldValue = deserialize(msg, ctx);
        result[fieldName] = fieldValue;
      }
      return result;
    } break;
    case SType.Array: {
      const result = [];
      while (true) {
        const fieldTag = dv.getUint8(ctx.cur);
        if (fieldTag === SType.End) {
          ctx.cur += 1;
          break
        }
        result.push(deserialize(msg, ctx));
      }
      return result;
    } break;
    case SType.Float: {
      const result = dv.getFloat64(ctx.cur, true);
      ctx.cur += 8;
      return result;
    } break;
    case SType.Bool: {
      const result = dv.getUint8(ctx.cur);
      ctx.cur += 1;
      if (result > 1) {
        throw new Error(`expected bool byte but got ${result}`);
      }
      return result !== 0;
    } break;
    case SType.String: {
      const length = dv.getFloat64(ctx.cur, true);
      ctx.cur += 8;
      const result = new TextDecoder().decode(msg.subarray(ctx.cur, ctx.cur + length));
      ctx.cur += length;
      return result;
    } break;
    default:
      throw new Error(`unknown value type ${tag}`);
  }
}

export function serialize(val: any): Uint8Array<ArrayBuffer> {
  const bytes: number[] = [];
  function serializeRecursive(val: any) {
    switch (Array.isArray(val) ? "array" : typeof val) {
      case "object": {
        bytes.push(SType.Object);
        for (const [name, value] of Object.entries(val)) {
          serializeRecursive(name);
          serializeRecursive(value);
        }
        bytes.push(SType.End);
      } break;
      case "array": {
        bytes.push(SType.Array);
        for (const elem of val) {
          serializeRecursive(elem);
        }
        bytes.push(SType.End);
      } break;
      case "number": {
        bytes.push(SType.Float);
        const buf = new ArrayBuffer(8);
        new DataView(buf).setFloat64(0, val, true);
        bytes.push(...new Uint8Array(buf));
      } break;
      case "string": {
        bytes.push(SType.String);
        const buf = new ArrayBuffer(8);
        new DataView(buf).setFloat64(0, val.length, true);
        bytes.push(...new Uint8Array(buf));
        bytes.push(...new TextEncoder().encode(val));
      } break;
      case "boolean": {
        bytes.push(SType.Bool);
        bytes.push(val ? 1 : 0);
      } break;
      default:
        throw new Error(`cannot encode values of type ${typeof val}`);
    }
  }
  serializeRecursive(val);
  return new Uint8Array(bytes);
}
