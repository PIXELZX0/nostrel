// Panel theme: the admin picks a logo, a background and two colours, and the
// rest of the palette is derived here so a light background still reads.

const hex = (v) => (/^#[0-9a-fA-F]{6}$/.test(v || "") ? v : null);
const rgb = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
const hexOf = ([r, g, b]) =>
  "#" + [r, g, b].map((n) => Math.round(n).toString(16).padStart(2, "0")).join("");

// perceived brightness, 0 (black) to 1 (white)
const luma = (h) => {
  const [r, g, b] = rgb(h);
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255;
};

const mix = (a, b, t) => hexOf(rgb(a).map((v, i) => v + (rgb(b)[i] - v) * t));

// applyTheme takes the theme half of /api/info and writes it onto :root.
function applyTheme(t) {
  const root = document.documentElement.style;

  const bg = hex(t.theme_bg_color);
  if (bg) {
    // white text on a dark background, black on a light one; every surface is
    // the background nudged that same way, so the whole panel follows along
    const fg = luma(bg) > 0.55 ? "#121417" : "#ffffff";
    root.setProperty("--bg", bg);
    root.setProperty("--fg", fg);
    root.setProperty("--surface", mix(bg, fg, 0.05));
    root.setProperty("--sidebar-bg", mix(bg, fg, 0.03));
    root.setProperty("--border", mix(bg, fg, 0.13));
    root.setProperty("--muted", mix(bg, fg, 0.55));
  }

  const accent = hex(t.theme_accent);
  if (accent) {
    root.setProperty("--accent", accent);
    root.setProperty("--accent-soft", accent + "1f");
    // button labels sit on the accent, so they need the readable one
    root.setProperty("--on-accent", luma(accent) > 0.55 ? "#121417" : "#ffffff");
  }

  if (t.theme_bg_image) root.setProperty("--bg-image", `url("${t.theme_bg_image}")`);
  else root.removeProperty("--bg-image");

  // the logo replaces the bolt in the sidebar. /api/info calls it "icon", the
  // admin's own settings object calls it "relay_icon".
  const logo = t.icon || t.relay_icon;
  const brand = document.querySelector(".brand .i, .brand .brand-logo");
  if (logo && brand) {
    const img = document.createElement("img");
    img.className = "brand-logo";
    img.src = logo;
    img.alt = "";
    brand.replaceWith(img);
  }
}
