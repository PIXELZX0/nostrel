// NIP-46 remote signing (bunker / nostrconnect).
//
// The user's key never reaches this page: we hold a throwaway key, talk to
// their signer over a relay in kind 24133 events, and ask it to sign for us.
// That is also why this runs in the browser and not on the relay — a
// server-side proxy could ask the signer to sign anything it liked.

import {
  conversationKey, finalizeEvent, generateSecretKey, getPublicKey,
  isNip04, nip04Decrypt, nip44Decrypt, nip44Encrypt,
} from "./nostr.js";

const KIND = 24133;
const REQUEST_TIMEOUT = 60_000;

class RemoteSigner {
  constructor(onStatus) {
    this.secretKey = generateSecretKey();
    this.clientPubkey = getPublicKey(this.secretKey);
    this.onStatus = onStatus || (() => {});
    this.sockets = [];
    this.pending = new Map();
    this.convKeys = new Map();
    this.remote = null;
    this.pubkey = null;
  }

  convKey(pubkey) {
    if (!this.convKeys.has(pubkey)) {
      this.convKeys.set(pubkey, conversationKey(this.secretKey, pubkey));
    }
    return this.convKeys.get(pubkey);
  }

  // open subscribes to every relay the signer offered. Requests go to all of
  // them: the signer only needs to be listening on one.
  async open(relays) {
    this.relays = [...new Set(relays)].filter((r) => r.startsWith("ws"));
    if (!this.relays.length) throw new Error("연결 문자열에 relay가 없습니다.");

    await Promise.all(this.relays.map((url) => new Promise((resolve) => {
      let socket;
      try {
        socket = new WebSocket(url);
      } catch {
        return resolve();
      }
      // one dead relay must not block a working one
      const done = setTimeout(resolve, 5000);
      socket.onopen = () => {
        clearTimeout(done);
        socket.send(JSON.stringify(["REQ", "nip46", {
          kinds: [KIND],
          "#p": [this.clientPubkey],
          since: Math.floor(Date.now() / 1000) - 10,
        }]));
        resolve();
      };
      socket.onerror = () => { clearTimeout(done); resolve(); };
      socket.onmessage = (msg) => this.onMessage(msg.data);
      this.sockets.push(socket);
    })));

    if (!this.sockets.some((s) => s.readyState === WebSocket.OPEN)) {
      throw new Error("서명자 relay에 연결하지 못했습니다.");
    }
  }

  async onMessage(raw) {
    let frame;
    try {
      frame = JSON.parse(raw);
    } catch {
      return;
    }
    if (frame[0] !== "EVENT" || frame[2]?.kind !== KIND) return;

    const event = frame[2];
    let payload;
    try {
      payload = isNip04(event.content)
        ? await nip04Decrypt(this.secretKey, event.pubkey, event.content)
        : nip44Decrypt(event.content, this.convKey(event.pubkey));
    } catch {
      return; // not for us, or a signer we have no key for
    }

    let message;
    try {
      message = JSON.parse(payload);
    } catch {
      return;
    }

    // A signer that answers "auth_url" wants the user to approve in a browser
    // window before it will sign anything.
    if (message.result === "auth_url") {
      this.onStatus({ authUrl: message.error });
      return;
    }

    const waiter = this.pending.get(message.id);
    if (waiter) {
      this.pending.delete(message.id);
      if (message.error) waiter.reject(new Error(message.error));
      else waiter.resolve({ result: message.result, from: event.pubkey });
      return;
    }
    // unsolicited: the nostrconnect handshake, where the signer speaks first
    this.onConnect?.({ result: message.result, from: event.pubkey, id: message.id });
  }

  async request(method, params, to = this.remote) {
    const id = crypto.randomUUID();
    const content = nip44Encrypt(
      JSON.stringify({ id, method, params }), this.convKey(to));
    const event = finalizeEvent(
      { kind: KIND, tags: [["p", to]], content }, this.secretKey);

    const frame = JSON.stringify(["EVENT", event]);
    let sent = false;
    for (const socket of this.sockets) {
      if (socket.readyState !== WebSocket.OPEN) continue;
      socket.send(frame);
      sent = true;
    }
    if (!sent) throw new Error("서명자 relay 연결이 끊겼습니다.");

    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error("서명자가 응답하지 않았습니다."));
      }, REQUEST_TIMEOUT);
      this.pending.set(id, {
        resolve: (value) => { clearTimeout(timer); resolve(value); },
        reject: (err) => { clearTimeout(timer); reject(err); },
      });
    });
  }

  async signEvent(event) {
    const { result } = await this.request("sign_event", [JSON.stringify(event)]);
    return typeof result === "string" ? JSON.parse(result) : result;
  }

  close() {
    for (const socket of this.sockets) socket.close();
    this.sockets = [];
    this.pending.clear();
  }
}

// connectBunker follows a bunker:// string the user pasted from their signer.
async function connectBunker(uri, onStatus) {
  const url = new URL(uri.trim());
  if (url.protocol !== "bunker:") throw new Error("bunker:// 로 시작하는 문자열이 필요합니다.");

  const remote = (url.hostname || url.pathname.replace(/^\/\//, "")).toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(remote)) throw new Error("bunker 주소의 pubkey가 올바르지 않습니다.");

  const signer = new RemoteSigner(onStatus);
  await signer.open(url.searchParams.getAll("relay"));
  signer.remote = remote;

  onStatus({ message: "서명자에 연결 요청을 보냈습니다. 지갑 앱에서 승인하세요." });
  const secret = url.searchParams.get("secret");
  await signer.request("connect", secret ? [remote, secret] : [remote]);

  const { result } = await signer.request("get_public_key", []);
  signer.pubkey = String(result).toLowerCase();
  return signer;
}

// startNostrconnect goes the other way: we publish an invitation and wait for
// the signer to open the connection.
function startNostrconnect({ relays, name, url, perms }, onStatus) {
  const signer = new RemoteSigner(onStatus);
  const secret = crypto.randomUUID();

  const params = new URLSearchParams();
  for (const relay of relays) params.append("relay", relay);
  params.set("secret", secret);
  if (name) params.set("name", name);
  if (url) params.set("url", url);
  if (perms) params.set("perms", perms);
  const uri = `nostrconnect://${signer.clientPubkey}?${params}`;

  const connected = (async () => {
    await signer.open(relays);
    const approval = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("연결 대기 시간이 지났습니다.")), 180_000);
      signer.onConnect = (message) => {
        // the signer proves it read our invitation by echoing the secret
        if (message.result !== secret && message.result !== "ack") return;
        clearTimeout(timer);
        resolve(message);
      };
    });

    signer.remote = approval.from;
    const { result } = await signer.request("get_public_key", []);
    signer.pubkey = String(result).toLowerCase();
    return signer;
  })();

  return { uri, connected, cancel: () => signer.close() };
}

window.nip46 = { connectBunker, startNostrconnect };
