const form = document.getElementById("composer");
const input = document.getElementById("q");
const log = document.getElementById("log");
const reading = document.getElementById("reading");
const starters = document.getElementById("starters");

const RULER_MIN = 0.2; // fixed scale, so rulers from different questions compare
const RULER_MAX = 0.6;
const turns = [];

starters.addEventListener("click", (e) => {
  if (e.target.tagName === "BUTTON") ask(e.target.textContent);
});
form.addEventListener("submit", (e) => {
  e.preventDefault();
  ask(input.value);
});
input.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    form.requestSubmit();
  }
});
input.addEventListener("input", () => {
  input.style.height = "auto";
  input.style.height = input.scrollHeight + "px";
});

async function post(path, question) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ question }),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

async function ask(raw) {
  const question = raw.trim();
  if (!question) return;
  input.value = "";
  input.style.height = "auto";
  starters.hidden = true;
  setBusy(true);

  const turn = { question, chunks: null, cited: [] };
  turns.push(turn);
  turn.el = el("li", { class: "turn" }, [
    el("p", { class: "question" }, question),
    el("div", { class: "body" }, [el("p", { class: "status" }, "Searching the docs…")]),
  ]);
  turn.el.addEventListener("click", (e) => {
    if (!e.target.closest("a")) select(turn);
  });
  log.append(turn.el);
  select(turn);
  turn.el.scrollIntoView({ behavior: "smooth", block: "start" });
  const body = turn.el.querySelector(".body");

  try {
    turn.chunks = (await post("/api/retrieve", question)).chunks;
    refresh(turn);
    body.replaceChildren(el("p", { class: "status" }, "Writing an answer from these passages…"));

    const res = await post("/api/answer", question);
    turn.cited = res.cited;
    const answer = el("div", { class: "answer" + (res.refused ? " refused" : "") });
    answer.innerHTML = res.html; // server-rendered Markdown; raw HTML from the model is escaped there
    body.replaceChildren(answer);
    wireCitations(turn, answer);
    refresh(turn);
  } catch (err) {
    body.replaceChildren(el("p", { class: "error" }, err.message));
  } finally {
    setBusy(false);
    input.focus();
  }
}

function setBusy(busy) {
  form.querySelector("button").disabled = busy;
}

function select(turn) {
  if (turns.selected === turn) return;
  turns.forEach((t) => t.el.classList.toggle("selected", t === turn));
  turns.selected = turn;
  renderReading(turn);
}

function refresh(turn) {
  if (turns.selected === turn) renderReading(turn);
  let inline = turn.el.querySelector(".inline-reading");
  if (!turn.chunks) return;
  if (!inline) {
    inline = el("details", { class: "inline-reading" }, [el("summary", {}, "What it read")]);
    turn.el.append(inline);
  }
  inline.replaceChildren(inline.firstChild, ...trace(turn));
}

function renderReading(turn) {
  const parts = [el("h2", {}, "What it read")];
  if (!turn.chunks) {
    parts.push(el("p", { class: "reading-empty" }, "Searching…"));
  } else {
    parts.push(el("p", { class: "reading-for" }, turn.question), ...trace(turn));
  }
  reading.replaceChildren(...parts);
}

// trace builds the ruler and the passage list for one turn.
function trace(turn) {
  const pct = (d) => (Math.min(Math.max(d, RULER_MIN), RULER_MAX) - RULER_MIN) / (RULER_MAX - RULER_MIN) * 100;
  const track = el("div", { class: "ruler-track" });
  for (const t of [0.3, 0.4, 0.5]) {
    track.append(el("div", { class: "tick", style: `left:${pct(t)}%` }, [el("span", {}, t.toFixed(1))]));
  }
  const lanes = [];
  for (const c of turn.chunks) {
    const x = pct(c.distance);
    let lane = lanes.findIndex((last) => x - last > 6);
    if (lane === -1) lane = lanes.length;
    lanes[lane] = x;
    const dot = el("span", {
      class: "dot" + (turn.cited.includes(c.n) ? " cited" : ""),
      style: `left:${turn.placed ? x : 0}%;bottom:${0.45 + lane * 1.55}rem`,
      "data-n": c.n,
      title: `Passage ${c.n}, distance ${c.distance.toFixed(3)}`,
    }, String(c.n));
    track.append(dot);
    if (!turn.placed) requestAnimationFrame(() => requestAnimationFrame(() => (dot.style.left = x + "%")));
  }
  track.style.height = `${1.2 + lanes.length * 1.55}rem`; // grow with stacked dots instead of overlapping the text above
  setTimeout(() => (turn.placed = true));
  const ruler = el("div", { class: "ruler" }, [
    track,
    el("div", { class: "ruler-ends" }, [el("span", {}, "Closer to your question"), el("span", {}, "Farther")]),
  ]);

  const list = el("ol", { class: "passages" }, turn.chunks.map((c) => {
    const cited = turn.cited.includes(c.n);
    return el("li", { class: "passage" + (cited ? " cited" : ""), "data-n": c.n }, [
      el("div", { class: "passage-head" }, [
        el("span", { class: "passage-n" }, String(c.n)),
        el("span", { class: "passage-title" }, c.pageTitle),
      ]),
      el("p", { class: "passage-path" }, c.headingPath || "Introduction"),
      el("p", { class: "passage-text" }, c.excerpt),
      el("div", { class: "passage-head" }, [
        el("span", { class: "passage-meta" }, `Distance ${c.distance.toFixed(3)}, ${cited ? "cited" : "not cited"}`),
        el("a", { href: c.url, target: "_blank", rel: "noopener" }, "Open on go.dev"),
      ]),
    ]);
  }));
  return [ruler, list];
}

// Hovering a citation lights up its passage and dot.
function wireCitations(turn, answer) {
  for (const a of answer.querySelectorAll(".cite")) {
    const light = (on) => {
      select(turn);
      document.querySelectorAll(`.dot[data-n="${a.dataset.n}"], .passage[data-n="${a.dataset.n}"]`)
        .forEach((x) => x.classList.toggle("lit", on));
      a.classList.toggle("lit", on);
    };
    a.addEventListener("mouseenter", () => light(true));
    a.addEventListener("mouseleave", () => light(false));
    a.addEventListener("focus", () => light(true));
    a.addEventListener("blur", () => light(false));
  }
}

function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v);
  if (typeof children === "string") node.textContent = children;
  else node.append(...children);
  return node;
}
