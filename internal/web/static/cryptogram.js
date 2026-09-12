(() => {
    const form = document.getElementById("cryptogram-guess-form");
    if (!form) return;

    const codeInput = document.getElementById("cryptogram-code");
    const letterInput = document.getElementById("cryptogram-letter");
    const focusInput = document.getElementById("cryptogram-focus");
    const nextFocusInput = document.getElementById("cryptogram-next-focus");
    const inputs = Array.from(document.querySelectorAll(".cipher-input"));

    const requestedFocus = new URLSearchParams(window.location.search).get("focus");
    if (requestedFocus !== null) {
        const input = inputs[Number(requestedFocus)];
        if (input && !input.readOnly) input.focus();
    }

    inputs.filter((input) => !input.readOnly).forEach((input) => {
        input.addEventListener("input", () => {
            const letter = input.value.toUpperCase().replace(/[^A-Z]/g, "").slice(0, 1);
            input.value = letter;
            if (!letter) return;

            const currentIndex = inputs.indexOf(input);
            const nextIndex = inputs.findIndex((candidate, index) =>
                index > currentIndex && !candidate.readOnly && candidate.dataset.code !== input.dataset.code,
            );
            codeInput.value = input.dataset.code;
            letterInput.value = letter;
            focusInput.value = currentIndex;
            nextFocusInput.value = nextIndex >= 0 ? nextIndex : currentIndex;
            form.requestSubmit();
        });
    });
})();
