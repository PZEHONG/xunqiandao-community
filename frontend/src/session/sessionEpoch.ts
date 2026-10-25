export type SessionEpoch = number;
let epoch: SessionEpoch = 0;
export function getSessionEpoch() { return epoch; }
export function bumpSessionEpoch() { return ++epoch; }
