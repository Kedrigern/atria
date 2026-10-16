// Rendering plugins (highlight.js, MathJax, Mermaid, note TOC), re-run after every htmx swap.
import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs";
mermaid.initialize({ startOnLoad: false, theme: "neutral" });

function initPlugins() {
    // 1. Code highlighting
    if (typeof hljs !== "undefined") {
        document
            .querySelectorAll("pre code:not(.language-mermaid)")
            .forEach((el) => {
                if (!el.dataset.highlighted) {
                    hljs.highlightElement(el);
                }
            });
    }

    // 2. MathJax
    if (typeof MathJax !== "undefined" && MathJax.typesetPromise) {
        MathJax.typesetPromise();
    }

    // 3. Mermaid diagrams
    const unrenderedMermaids = [];
    document
        .querySelectorAll("code.language-mermaid")
        .forEach((block) => {
            const pre = block.parentElement;
            // Ochrana, abychom nezpracovávali jeden graf dvakrát
            if (!pre.hasAttribute("data-mermaid-processed")) {
                pre.classList.add("mermaid");
                pre.setAttribute("data-mermaid-processed", "true");
                pre.textContent = block.textContent; // Přepis na čistý text pro Mermaid
                unrenderedMermaids.push(pre);
            }
        });

    // Spustíme Mermaid pouze nad těmi uzly, které ještě nebyly vykresleny
    if (unrenderedMermaids.length > 0) {
        try {
            mermaid.run({ nodes: unrenderedMermaids });
        } catch (e) {
            console.warn("Mermaid error:", e);
        }
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
