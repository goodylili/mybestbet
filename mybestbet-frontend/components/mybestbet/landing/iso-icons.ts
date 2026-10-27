/* Isometric block icons for the "how it works" cards, drawn as SVG markup. */

type Palette = [string, string, string]

const C: Record<"b" | "g" | "w", Palette> = {
  b: ["#7d97ff", "#3d5bff", "#2340d4"],
  g: ["#f4f5f8", "#dcdfe8", "#c4c9d7"],
  w: ["#ffffff", "#e9ecf3", "#d3d8e4"],
}
const SIN = 0.866
const K = 9

const P = (x: number, y: number, z: number) => [60 + (x - y) * SIN * K, 52 + (x + y) * 0.5 * K - z * K]

const pts = (a: number[][]) => a.map((p) => { const q = P(p[0], p[1], p[2]); return q[0].toFixed(1) + "," + q[1].toFixed(1) }).join(" ")

function box(x: number, y: number, z: number, w: number, d: number, h: number, c: Palette) {
  return '<polygon fill="' + c[0] + '" points="' + pts([[x, y, z + h], [x + w, y, z + h], [x + w, y + d, z + h], [x, y + d, z + h]]) + '"/>' +
    '<polygon fill="' + c[1] + '" points="' + pts([[x, y + d, z + h], [x + w, y + d, z + h], [x + w, y + d, z], [x, y + d, z]]) + '"/>' +
    '<polygon fill="' + c[2] + '" points="' + pts([[x + w, y, z + h], [x + w, y + d, z + h], [x + w, y + d, z], [x + w, y, z]]) + '"/>'
}

function shadow() {
  const c = P(2.5, 2.5, 0)
  return '<ellipse cx="' + c[0] + '" cy="' + (c[1] + 14) + '" rx="40" ry="11" fill="rgb(20 30 80 / .12)"/>'
}

function cyl(cx: number, cy: number, r: number, h: number, z: number, c: Palette) {
  const p = P(cx, cy, z), t = P(cx, cy, z + h), rx = r * K * 1.05, ry = rx * 0.52
  return '<path fill="' + c[1] + '" d="M' + (p[0] - rx) + " " + p[1] + " L" + (t[0] - rx) + " " + t[1] + " A" + rx + " " + ry + " 0 0 0 " + (t[0] + rx) + " " + t[1] + " L" + (p[0] + rx) + " " + p[1] + " A" + rx + " " + ry + " 0 0 1 " + (p[0] - rx) + " " + p[1] + ' Z"/>' +
    '<path fill="' + c[2] + '" opacity=".55" d="M' + p[0] + " " + (p[1] + ry) + " L" + t[0] + " " + (t[1] + ry) + " A" + rx + " " + ry + " 0 0 0 " + (t[0] + rx) + " " + t[1] + " L" + (p[0] + rx) + " " + p[1] + " A" + rx + " " + ry + " 0 0 1 " + p[0] + " " + (p[1] + ry) + ' Z"/>' +
    '<ellipse cx="' + t[0] + '" cy="' + t[1] + '" rx="' + rx + '" ry="' + ry + '" fill="' + c[0] + '"/>' +
    '<ellipse cx="' + t[0] + '" cy="' + t[1] + '" rx="' + rx * 0.62 + '" ry="' + ry * 0.62 + '" fill="none" stroke="' + c[2] + '" stroke-opacity=".5" stroke-width="1.4"/>'
}

const ICONS = {
  radar: () => shadow() + box(0, 0, 0, 5, 5, 1.1, C.g) + box(0.4, 0.4, 1.1, 4.2, 4.2, 1.1, C.w) + box(0.8, 0.8, 2.2, 3.4, 3.4, 1.1, C.b) + cyl(2.5, 2.5, 0.55, 1.6, 3.3, C.w),
  bars: () => shadow() + box(0, 0, 0, 5, 5, 0.7, C.g) + box(0.5, 3.2, 0.7, 1.2, 1.2, 1.6, C.w) + box(1.9, 1.9, 0.7, 1.2, 1.2, 2.8, C.w) + box(3.3, 0.6, 0.7, 1.2, 1.2, 4.4, C.b),
  bolt: () => {
    const z = 2.21
    return shadow() + box(0.4, 0.4, 0, 4.2, 4.2, 2.2, C.b) +
      '<polygon fill="#fff" points="' + pts([[2.9, 0.9, z], [1.3, 2.7, z], [2.4, 2.7, z], [1.7, 4.1, z], [3.7, 2.1, z], [2.6, 2.1, z]]) + '"/>'
  },
  coins: () => shadow() + cyl(1.6, 3.2, 1.25, 0.55, 0, C.g) + cyl(1.6, 3.2, 1.25, 0.55, 0.55, C.g) + cyl(3.4, 1.6, 1.25, 0.55, 0, C.w) + cyl(3.4, 1.6, 1.25, 0.55, 0.55, C.w) +
    cyl(3.4, 1.6, 1.25, 0.55, 1.1, C.b) + cyl(3.4, 1.6, 1.25, 0.55, 1.65, C.b) + cyl(3.4, 1.6, 1.25, 0.55, 2.2, C.b),
}

export type IsoIcon = keyof typeof ICONS

export const isoSvg = (kind: IsoIcon) => '<svg viewBox="0 0 120 112" xmlns="http://www.w3.org/2000/svg">' + ICONS[kind]() + "</svg>"
