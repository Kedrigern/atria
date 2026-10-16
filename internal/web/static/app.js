// Atria page behaviour. Lives in a static file (not inline <script> or on*
// attributes) so the Content-Security-Policy can forbid inline scripts.
// Elements opt into behaviour with data-* attributes, handled by the
// delegated listeners at the bottom of this file.

if ("serviceWorker" in navigator) {
    window.addEventListener("load", function () {
        navigator.serviceWorker.register("/sw.js");
    });
}

document.addEventListener("DOMContentLoaded", function () {
    document.body.addEventListener(
        "htmx:configRequest",
        function (e) {
            const token = document.querySelector(
                'meta[name="csrf-token"]',
            );
            if (token)
                e.detail.headers["X-CSRF-Token"] =
                    token.getAttribute("content");
        },
    );
});

const sidebar = document.getElementById("sidebar");
const sidebarBackdrop = document.getElementById("sidebar-backdrop");

function toggleSidebar() {
    if (!sidebar || !sidebarBackdrop) return;
    sidebar.classList.toggle("open");
    sidebarBackdrop.classList.toggle("open");
}

function closeSidebar() {
    if (!sidebar || !sidebarBackdrop) return;
    sidebar.classList.remove("open");
    sidebarBackdrop.classList.remove("open");
}

const menuToggle = document.getElementById("menu-toggle");
if (menuToggle) menuToggle.addEventListener("click", toggleSidebar);
if (sidebarBackdrop) sidebarBackdrop.addEventListener("click", closeSidebar);

document.body.addEventListener("htmx:afterSettle", function () {
    const currentPath = window.location.pathname;

    document.querySelectorAll(".nav-link").forEach((link) => {
        link.classList.remove("active");
        const href = link.getAttribute("href");

        if (
            href === currentPath ||
            (href !== "/" && currentPath.startsWith(href))
        ) {
            link.classList.add("active");
        }
    });

    if (window.innerWidth <= 1024) closeSidebar();
});

document.body.dispatchEvent(new Event("htmx:afterSettle"));

document.body.addEventListener("htmx:afterSettle", function () {
    const title = document.querySelector(
        "#main-content [data-mobile-title]",
    );
    const headerTitle = document.getElementById(
        "dynamic-header-title",
    );

    if (headerTitle) {
        headerTitle.innerText = title
            ? title.getAttribute("data-mobile-title")
            : "ATRIA";
    }
});

document.body.addEventListener(
    "htmx:responseError",
    function (evt) {
        const errorMsg =
            evt.detail.xhr.responseText || "Neznámá chyba serveru";

        alert("❌ Oops, něco se pokazilo:\n\n" + errorMsg);

        const el = evt.detail.elt;
        if (el.tagName === "BUTTON") {
            el.innerText = "Error (Zkusit znovu)";
        }
    },
);

document.body.addEventListener("click", function (evt) {
    const flash = evt.target.closest("[data-flash-close]");
    if (!flash) return;

    const container = flash.closest(".flash-message");
    if (container) {
        container.remove();
    }
});

function setNoteEditing(editing) {
    const detail = document.getElementById("note-detail");
    if (detail) detail.classList.toggle("is-editing", editing);
    document.getElementById("note-view").style.display = editing ? "none" : "block";
    document.getElementById("note-edit").style.display = editing ? "block" : "none";

    if (editing) {
        const textarea = document.querySelector("#note-edit textarea");
        if (textarea) textarea.focus();
    }
}

// --- Delegated data-* actions -------------------------------------------

document.addEventListener("click", function (evt) {
    const el = evt.target.closest(
        "[data-note-editing], [data-copy], [data-tree-open], [data-toggle-inline], [data-hide-inline], [data-scroll-top], [data-ebook-view], [data-open-dialog]",
    );
    if (!el) return;

    if (el.hasAttribute("data-note-editing")) {
        evt.preventDefault();
        setNoteEditing(el.dataset.noteEditing === "true");
    } else if (el.hasAttribute("data-copy")) {
        // data-copy-origin prefixes the value with the site origin (public links).
        const value = el.hasAttribute("data-copy-origin")
            ? window.location.origin + el.dataset.copy
            : el.dataset.copy;
        navigator.clipboard.writeText(value);
    } else if (el.hasAttribute("data-tree-open")) {
        const open = el.dataset.treeOpen === "true";
        document.querySelectorAll(".note-tree details").forEach((d) => (d.open = open));
    } else if (el.hasAttribute("data-toggle-inline")) {
        evt.preventDefault();
        const content = el.closest(".list-item")?.querySelector(".inline-content");
        if (content) content.style.display = content.style.display === "none" ? "block" : "none";
    } else if (el.hasAttribute("data-hide-inline")) {
        const content = el.closest(".inline-content");
        if (content) content.style.display = "none";
    } else if (el.hasAttribute("data-scroll-top")) {
        document.getElementById("main-content").scrollTo({ top: 0, behavior: "smooth" });
    } else if (el.hasAttribute("data-ebook-view")) {
        const library = document.querySelector("[data-ebook-library]");
        if (library) library.dataset.view = el.dataset.ebookView;
        try {
            localStorage.setItem("atria-ebook-view", el.dataset.ebookView);
        } catch (e) {
            // Storage unavailable (private mode); the view still switches.
        }
    } else if (el.hasAttribute("data-open-dialog")) {
        const dialog = document.getElementById(el.dataset.openDialog);
        if (dialog) dialog.showModal();
    }
});

// Inputs/selects with data-auto-submit submit their form on change.
// requestSubmit() fires a real submit event, so htmx-enhanced forms work too.
document.addEventListener("change", function (evt) {
    const el = evt.target.closest("[data-auto-submit]");
    if (el && el.form) el.form.requestSubmit();
});

// Forms with data-confirm ask before submitting. Capture phase, so a
// cancelled submit never reaches htmx.
document.addEventListener(
    "submit",
    function (evt) {
        const form = evt.target.closest("[data-confirm]");
        if (form && !window.confirm(form.dataset.confirm)) {
            evt.preventDefault();
            evt.stopImmediatePropagation();
        }
    },
    true,
);
