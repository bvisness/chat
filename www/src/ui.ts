import { E } from "./dom";
import { ChatEvent, deserialize, EType, RType, serialize } from "./protocol";

export class Client {
  container: HTMLDivElement;

  constructor() {
    this.container = E("div", [], [
      E("div", ["chat"], [
        E("div", ["log"], []),
        E("input", [], el => { el.type = "text"; }, []),
      ]),
      E("table", [], [
        E("thead", [], [
          E("tr", [], [
            E("th", [], ["Sent"]),
            E("th", [], ["Received"]),
            E("th", [], ["Preview"]),
          ]),
        ]),
        E("tbody", [], []),
      ]),
    ]);
  }

  async connect() {
    const input = this.container.querySelector("input")!;
    const log = this.container.querySelector(".log")!;

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

    let latestServerSN = -1;
    const send = (msg: ChatEvent) => {
      console.debug("Client event", msg);
      const msgBytes = serialize(msg);
      const row = document.createElement("tr");
      row.appendChild(this.createLogCell(msgBytes));
      row.appendChild(document.createElement("td"));
      row.appendChild(this.createHexView(msgBytes));
      this.container.querySelector("tbody")!.appendChild(row);
      ws.send(msgBytes);
    }
    const receive = (msgBytes: Uint8Array<ArrayBuffer>) => {
      const row = document.createElement("tr");
      row.appendChild(document.createElement("td"));
      row.appendChild(this.createLogCell(msgBytes));
      row.appendChild(this.createHexView(msgBytes));
      this.container.querySelector("tbody")!.appendChild(row);

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
  }

  createLogCell(msgBytes: Uint8Array) {
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

  createHexView(msgBytes: Uint8Array) {
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
}
