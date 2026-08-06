// Nostr primitives the browser needs to talk to a remote signer: key
// generation, event signing, and the NIP-44 encryption NIP-46 rides on.
//
// The heavy lifting is done by vendored, audited @noble libraries; everything
// here is the thin NIP-specific layer on top.

import { schnorr, secp256k1 } from "./vendor/curves/secp256k1.js";
import { extract, expand } from "./vendor/hashes/hkdf.js";
import { hmac } from "./vendor/hashes/hmac.js";
import { sha256 } from "./vendor/hashes/sha2.js";
import { chacha20 } from "./vendor/ciphers/chacha.js";

const utf8 = new TextEncoder();
const fromUtf8 = new TextDecoder();

export const toHex = (bytes) =>
  [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
export const fromHex = (hex) =>
  Uint8Array.from(hex.match(/.{2}/g).map((b) => parseInt(b, 16)));

const concat = (...arrays) => {
  const out = new Uint8Array(arrays.reduce((n, a) => n + a.length, 0));
  let at = 0;
  for (const a of arrays) { out.set(a, at); at += a.length; }
  return out;
};

const toBase64 = (bytes) => btoa(String.fromCharCode(...bytes));
const fromBase64 = (text) => Uint8Array.from(atob(text), (c) => c.charCodeAt(0));

// --- keys and events ---

export const generateSecretKey = () => schnorr.utils.randomPrivateKey();
export const getPublicKey = (secretKey) => toHex(schnorr.getPublicKey(secretKey));

// serialize is the exact array NIP-01 hashes to produce an event id.
const serialize = (evt) =>
  JSON.stringify([0, evt.pubkey, evt.created_at, evt.kind, evt.tags, evt.content]);

export function finalizeEvent(event, secretKey) {
  const evt = {
    ...event,
    pubkey: getPublicKey(secretKey),
    created_at: event.created_at ?? Math.floor(Date.now() / 1000),
    tags: event.tags ?? [],
    content: event.content ?? "",
  };
  evt.id = toHex(sha256(utf8.encode(serialize(evt))));
  evt.sig = toHex(schnorr.sign(fromHex(evt.id), secretKey));
  return evt;
}

// --- NIP-44 v2 ---

// conversationKey is derived once per (us, them) pair and reused for every
// message, so it is worth caching by the caller.
export function conversationKey(secretKey, publicKeyHex) {
  const shared = secp256k1.getSharedSecret(secretKey, "02" + publicKeyHex);
  return extract(sha256, shared.subarray(1, 33), utf8.encode("nip44-v2"));
}

// messageKeys stretches the conversation key with the per-message nonce.
function messageKeys(convKey, nonce) {
  const keys = expand(sha256, convKey, nonce, 76);
  return {
    chachaKey: keys.subarray(0, 32),
    chachaNonce: keys.subarray(32, 44),
    hmacKey: keys.subarray(44, 76),
  };
}

// Plaintext is padded to a power-of-two-ish length so the ciphertext size
// leaks as little as possible about the message.
function paddedLength(length) {
  if (length <= 32) return 32;
  const nextPower = 1 << (Math.floor(Math.log2(length - 1)) + 1);
  const chunk = nextPower <= 256 ? 32 : nextPower / 8;
  return chunk * (Math.floor((length - 1) / chunk) + 1);
}

function pad(plaintext) {
  const bytes = utf8.encode(plaintext);
  if (bytes.length < 1 || bytes.length > 65535) throw new Error("invalid plaintext length");
  const header = new Uint8Array([bytes.length >> 8, bytes.length & 0xff]);
  return concat(header, bytes, new Uint8Array(paddedLength(bytes.length) - bytes.length));
}

function unpad(padded) {
  const length = (padded[0] << 8) | padded[1];
  const bytes = padded.subarray(2, 2 + length);
  if (length < 1 || bytes.length !== length || padded.length !== 2 + paddedLength(length)) {
    throw new Error("invalid padding");
  }
  return fromUtf8.decode(bytes);
}

export function nip44Encrypt(plaintext, convKey, nonce = crypto.getRandomValues(new Uint8Array(32))) {
  const { chachaKey, chachaNonce, hmacKey } = messageKeys(convKey, nonce);
  const ciphertext = chacha20(chachaKey, chachaNonce, pad(plaintext));
  const mac = hmac(sha256, hmacKey, concat(nonce, ciphertext));
  return toBase64(concat(new Uint8Array([2]), nonce, ciphertext, mac));
}

export function nip44Decrypt(payload, convKey) {
  if (payload[0] === "#") throw new Error("unsupported nip-44 encryption version");
  const data = fromBase64(payload);
  if (data[0] !== 2) throw new Error("unknown nip-44 version " + data[0]);
  if (data.length < 99 || data.length > 65603) throw new Error("invalid payload length");

  const nonce = data.subarray(1, 33);
  const ciphertext = data.subarray(33, data.length - 32);
  const mac = data.subarray(data.length - 32);
  const { chachaKey, chachaNonce, hmacKey } = messageKeys(convKey, nonce);

  const expected = hmac(sha256, hmacKey, concat(nonce, ciphertext));
  if (toHex(expected) !== toHex(mac)) throw new Error("invalid mac");

  return unpad(chacha20(chachaKey, chachaNonce, ciphertext));
}

// --- NIP-04 (legacy, decrypt only) ---

// Older signers still answer in NIP-04. Its payloads are distinguishable by
// the "?iv=" suffix, so we can accept both without guessing.
export const isNip04 = (payload) => payload.includes("?iv=");

export async function nip04Decrypt(secretKey, publicKeyHex, payload) {
  const [body, iv] = payload.split("?iv=");
  const shared = secp256k1.getSharedSecret(secretKey, "02" + publicKeyHex).subarray(1, 33);
  const key = await crypto.subtle.importKey("raw", shared, { name: "AES-CBC" }, false, ["decrypt"]);
  const plain = await crypto.subtle.decrypt(
    { name: "AES-CBC", iv: fromBase64(iv) }, key, fromBase64(body));
  return fromUtf8.decode(new Uint8Array(plain));
}

export async function nip04Encrypt(secretKey, publicKeyHex, plaintext) {
  const shared = secp256k1.getSharedSecret(secretKey, "02" + publicKeyHex).subarray(1, 33);
  const key = await crypto.subtle.importKey("raw", shared, { name: "AES-CBC" }, false, ["encrypt"]);
  const iv = crypto.getRandomValues(new Uint8Array(16));
  const cipher = await crypto.subtle.encrypt(
    { name: "AES-CBC", iv }, key, utf8.encode(plaintext));
  return toBase64(new Uint8Array(cipher)) + "?iv=" + toBase64(iv);
}
