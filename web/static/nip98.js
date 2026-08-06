// NIP-98 HTTP auth, shared by the public page and the admin panel.

async function sha256hex(text) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// Sign a kind 27235 event naming this exact URL, method and body.
// The nonce keeps two identical requests in the same second from producing the
// same event id, which the server's replay cache would reject.
//
// `signer` is anything with signEvent(): the NIP-07 extension by default, or a
// NIP-46 remote signer.
async function nip98Header(path, method, body, signer = window.nostr) {
  const tags = [["u", location.origin + path], ["method", method], ["nonce", crypto.randomUUID()]];
  if (body) tags.push(["payload", await sha256hex(body)]);
  const signed = await signer.signEvent({
    kind: 27235,
    created_at: Math.floor(Date.now() / 1000),
    tags,
    content: "",
  });
  return "Nostr " + btoa(JSON.stringify(signed));
}
