// Rendering plugins for notes and articles: code highlighting (highlight.js),
// formulas (MathJax), diagrams (Mermaid) and the note table of contents.
// Re-run after every htmx swap.
//
// The libraries are large, and Atria is often read on e-ink readers with slow
// processors (docs/architecture/ui.md, section 8). Each library is therefore
// downloaded only when the page contains something it renders.

const HLJS = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0";
const MATHJAX = "https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js";
const MERMAID = "https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs";

// Formula markers: $…$, $$…$$, \(…\) and \[…\].
const MATH_MARKERS = /\$\$|\$[^$\n]+\$|\\\(|\\\[/;

const scripts = new Map();

function loadScript(src) {
    if (!scripts.has(src)) {
        scripts.set(
            src,
            new Promise((resolve, reject) => {
                const script = document.createElement("script");
                script.src = src;
                script.async = true;
                script.onload = resolve;
                script.onerror = () => reject(new Error(`failed to load ${src}`));
                document.head.appendChild(script);
            }),
        );
    }
    return scripts.get(src);
}

function loadStylesheet(href, media) {
    if (document.querySelector(`link[href="${href}"]`)) return;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    if (media) link.media = media;
    document.head.appendChild(link);
}

async function highlightCode() {
    const blocks = [...document.querySelectorAll("pre code:not(.language-mermaid)")].filter((el) => !el.dataset.highlighted);
    if (blocks.length === 0) return;
    loadStylesheet(`${HLJS}/styles/github-dark.min.css`, "(prefers-color-scheme: dark)");
    loadStylesheet(`${HLJS}/styles/github.min.css`, "(prefers-color-scheme: light)");
    await loadScript(`${HLJS}/highlight.min.js`);
    blocks.forEach((el) => window.hljs.highlightElement(el));
}

// Formulas are only rendered inside note content (data-render-math), so that
// prices such as "$5 and $10" in articles or RSS items stay plain text.
async function typesetMath() {
    const containers = [...document.querySelectorAll("[data-render-math]")].filter((el) => MATH_MARKERS.test(el.textContent));
    if (containers.length === 0) return;
    if (!window.MathJax || !window.MathJax.typesetPromise) {
        window.MathJax = {
            tex: { inlineMath: [["$", "$"], ["\\(", "\\)"]] },
            startup: { typeset: false }, // typeset only the containers below
        };
        await loadScript(MATHJAX);
        await window.MathJax.startup.promise;
    }
    await window.MathJax.typesetPromise(containers);
}

let mermaid = null;

async function renderDiagrams() {
    const nodes = [];
    document.querySelectorAll("code.language-mermaid").forEach((block) => {
        const pre = block.parentElement;
        if (!pre.hasAttribute("data-mermaid-processed")) {
            pre.classList.add("mermaid");
            pre.setAttribute("data-mermaid-processed", "true");
            pre.textContent = block.textContent; // Mermaid reads the plain text
            nodes.push(pre);
        }
    });
    if (nodes.length === 0) return;
    if (!mermaid) {
        mermaid = (await import(MERMAID)).default;
        mermaid.initialize({ startOnLoad: false, theme: "neutral" });
    }
    await mermaid.run({ nodes });
}

function initPlugins() {
    for (const render of [highlightCode, typesetMath, renderDiagrams]) {
        render().catch((error) => console.warn(error));
    }

    // 4. Dynamické vygenerování Obsahu (TOC) pro poznámky
    const noteView = document.getElementById("note-view");
    // Ujistíme se, že jsme na detailu poznámky a že TOC ještě neexistuje
    if (
        noteView &&
        !document.getElementById("toc-container-mobile")
    ) {
        const headings = noteView.querySelectorAll("h2, h3");

        if (headings.length > 0) {
            const details = document.createElement("details");
            details.id = "toc-container";
            // Nastavíme podobný design jako mají detaily v RSS triage
            details.style.background = "var(--background-alt)";
            details.style.padding = "10px 15px";
            details.style.marginBottom = "25px";
            details.style.borderRadius = "6px";
            details.style.border = "1px solid var(--border)";

            const summary = document.createElement("summary");
            summary.textContent = "Obsah";
            summary.style.fontWeight = "bold";
            summary.style.color = "var(--text-heading)";
            summary.style.margin = "0";
            details.appendChild(summary);

            const ul = document.createElement("ul");
            ul.style.marginTop = "15px";
            ul.style.marginBottom = "5px";
            ul.style.paddingLeft = "20px";
            ul.style.listStyleType = "none"; // Čistší vzhled bez teček

            headings.forEach((h) => {
                const li = document.createElement("li");
                li.style.marginBottom = "8px";

                // H3 lehce odsadíme a zmenšíme, aby vznikla vizuální hierarchie
                if (h.tagName === "H3") {
                    li.style.marginLeft = "20px";
                    li.style.fontSize = "0.95em";
                }

                const a = document.createElement("a");
                a.href = "#" + h.id;
                a.textContent = h.innerText;
                a.style.color = "var(--text-main)";
                a.style.textDecoration = "none";

                // Hover efekt přes JS, ať neplevelíme CSS soubor
                a.onmouseover = () => {
                    a.style.textDecoration = "underline";
                    a.style.color = "var(--links)";
                };
                a.onmouseout = () => {
                    a.style.textDecoration = "none";
                    a.style.color = "var(--text-main)";
                };

                li.appendChild(a);
                ul.appendChild(li);
            });

            details.appendChild(ul);

            details.removeAttribute("id");
            const desktopTOC = details.cloneNode(true);
            desktopTOC.id = "toc-container-desktop";
            desktopTOC.classList.add("toc-desktop");
            desktopTOC.open = true; // Na desktopu rovnou rozbaleno

            const mobileTOC = details.cloneNode(true);
            mobileTOC.id = "toc-container-mobile";
            mobileTOC.classList.add("toc-mobile");

            // 1. Vložíme desktopovou verzi do sidebaru (pokud cíl existuje)
            const sidebarTarget =
                document.getElementById("toc-sidebar-target");
            if (sidebarTarget) {
                sidebarTarget.appendChild(desktopTOC);
            }

            // 2. Vložíme mobilní verzi těsně nad obsah poznámky
            noteView.parentNode.insertBefore(mobileTOC, noteView);
        }
    }
}

// Spustíme okamžitě pro první načtení stránky přes tvrdé URL
// (modul garantuje, že v tento moment je už DOM stránky hotový)
initPlugins();

// A navěsíme posluchače na HTMX pro všechna další "měkká" načtení (prokliky v menu)
document.body.addEventListener("htmx:afterSettle", initPlugins);
