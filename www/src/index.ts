import { ChatEvent, EType, RType, SType } from "./protocol";

function deserialize(msg: Uint8Array, ctx = { cur: 0 }): any {
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

function serialize(val: any): Uint8Array<ArrayBuffer> {
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

async function test(container: HTMLElement) {
  const input = container.querySelector("input")!;
  const log = container.querySelector(".log")!;

  const {
    promise: wsReady,
    resolve: resolveWsReady,
    reject: rejectWsReady,
  } = Promise.withResolvers<void>();
  const ws = new WebSocket("ws://localhost:8667/api/events");
  ws.binaryType = "arraybuffer";

  ws.addEventListener("open", () => {
    resolveWsReady();
  });
  ws.addEventListener("error", err => {
    console.error("websocket error", err);
    rejectWsReady(err);
  });

  function createLogCell(msgBytes: Uint8Array) {
    const td = document.createElement("td");
    const pre = document.createElement("pre");
    for (let i = 0; i < msgBytes.length; i++) {
      if (i > 0 && i % 16 === 0) {
        pre.innerHTML += "\n";
      } else if (i > 0) {
        pre.innerHTML += " ";
      }
      pre.innerHTML += msgBytes[i].toString(16).padStart(2, "0");
    }
    td.appendChild(pre);
    return td;
  }
  function createHexView(msgBytes: Uint8Array) {
    const td = document.createElement("td");
    const pre = document.createElement("pre");
    for (let r = 0; r <= Math.floor(msgBytes.length / 16); r++) {
      if (r > 0) {
        pre.innerHTML += "\n";
      }
      pre.innerHTML += "| ";
      for (let i = 0; i < 16; i++) {
        const b = msgBytes[16 * r + i];
        if (b === undefined) {
          pre.innerHTML += " ";
        } else if (0x20 <= b && b <= 0x7E) {
          pre.innerHTML += String.fromCharCode(b);
        } else {
          pre.innerHTML += ".";
        }
      }
      pre.innerHTML += " |";
    }
    td.appendChild(pre);
    return td;
  }

  let latestServerSN = -1;
  function send(msg: ChatEvent) {
    console.debug("Client event", msg);
    const msgBytes = serialize(msg);
    const row = document.createElement("tr");
    row.appendChild(createLogCell(msgBytes));
    row.appendChild(document.createElement("td"));
    row.appendChild(createHexView(msgBytes));
    container.querySelector("tbody")!.appendChild(row);
    ws.send(msgBytes);
  }
  function receive(msgBytes: Uint8Array<ArrayBuffer>) {
    const row = document.createElement("tr");
    row.appendChild(document.createElement("td"));
    row.appendChild(createLogCell(msgBytes));
    row.appendChild(createHexView(msgBytes));
    container.querySelector("tbody")!.appendChild(row);

    const event = deserialize(msgBytes);
    console.debug("Server event", event);

    switch (event.type) {
      case EType.Record: {
        const record = event.record;

        console.log(`Latest server SN is ${record.sn} (was ${latestServerSN})`);
        latestServerSN = record.sn;
        // TODO(ben): ACK periodically while receiving normal messages, before receiving
        // a SYN

        switch (record.type) {
        case RType.Message: {
          const row = document.createElement("div");
          row.innerText = record.text;
          log.appendChild(row);
          row.scrollIntoView({ behavior: "instant", block: "end" });
        } break;
        default:
          console.error(`unknown record type ${record.type}`);
        }
      } break;

      case EType.SYN: {
        send({ type: EType.ACK, sn: latestServerSN });
      } break;

      case EType.Error: {
        console.error(event.message);
      } break;

      default:
        console.error(`unknown event type ${event.type}`);
    }
  }

  ws.addEventListener("message", msg => {
    receive(new Uint8Array(msg.data));
  });

  input.addEventListener("keydown", e => {
    if (e.key === "Enter") {
      let text = input.value.trim();
      if (text.length > 0) {
        send({
          type: EType.Record,
          record: {
            type: RType.Message,
            text: text,
          },
        });
        input.value = "";
      }
    }
  });
  
  await wsReady;
  send({
    type: EType.SYN,
    sn: -1,
  });
  // setTimeout(() => ws.close(1000), 2000);
};

//@ts-ignore
test(window.one);
//@ts-ignore
setTimeout(() => test(window.two), 500);
