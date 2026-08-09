document.addEventListener("htmx:afterSwap", initializeEbookLibrary);
document.addEventListener("DOMContentLoaded", initializeEbookLibrary);

function initializeEbookLibrary() {
    const library = document.querySelector("[data-ebook-library]");
    if (!library || library.dataset.initialized) return;
    library.dataset.initialized = "true";
    library.dataset.view = localStorage.getItem("atria-ebook-view") === "tiles" ? "tiles" : "list";
}
