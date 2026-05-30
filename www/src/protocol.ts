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