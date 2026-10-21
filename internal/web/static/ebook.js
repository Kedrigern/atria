document.addEventListener("htmx:afterSwap", initializeEbookLibrary);
document.addEventListener("DOMContentLoaded", initializeEbookLibrary);

function initializeEbookLibrary() {
    const library = document.querySelector("[data-ebook-library]");
    if (!library || library.dataset.initialized) return;
    library.dataset.initialized = "true";
    let saved = null;
    try {
        saved = localStorage.getItem("atria-ebook-view");
    } catch (e) {
        // Storage unavailable (private mode); use the list view.
    }
    library.dataset.view = saved === "tiles" ? "tiles" : "list";
    markEbookView(library.dataset.view);
}

// The pressed button shows the active view with a border and bold text.
function markEbookView(view) {
    document.querySelectorAll("[data-ebook-view]").forEach((button) => {
        button.setAttribute("aria-pressed", String(button.dataset.ebookView === view));
    });
}
