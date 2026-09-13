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

    async function refreshFeedback() {
        const response = await fetch(window.location.href, { headers: { Accept: "text/html" } });
        if (!response.ok) return;
        const documentFragment = new DOMParser().parseFromString(await response.text(), "text/html");
        const currentFeedback = document.getElementById("cryptogram-feedback");
        const updatedFeedback = documentFragment.getElementById("cryptogram-feedback");
        if (currentFeedback && updatedFeedback) currentFeedback.replaceWith(updatedFeedback);
    }

    inputs.filter((input) => !input.readOnly).forEach((input) => {
        input.addEventListener("input", async () => {
            input.setCustomValidity("");
            const letter = input.value.toUpperCase().replace(/[^A-Z]/g, "").slice(0, 1);
            input.value = letter;
            if (!letter || input.dataset.submitting) return;

            const currentIndex = inputs.indexOf(input);
            const nextInput = inputs.find((candidate, index) =>
                index > currentIndex && !candidate.readOnly && candidate.dataset.code !== input.dataset.code,
            );
            codeInput.value = input.dataset.code;
            letterInput.value = letter;
            focusInput.value = currentIndex;
            nextFocusInput.value = nextInput ? inputs.indexOf(nextInput) : currentIndex;
            input.dataset.submitting = "true";

            try {
                const response = await fetch(form.action, {
                    method: "POST",
                    headers: { Accept: "application/json" },
                    body: new URLSearchParams(new FormData(form)),
                });
                const result = await response.json();
                if (!response.ok) throw new Error(result.error || "Failed to save guess");
                if (!result.success) {
                    input.value = "";
                    input.focus();
                    return;
                }

                inputs.filter((candidate) => candidate.dataset.code === input.dataset.code).forEach((candidate) => {
                    candidate.value = letter;
                    candidate.readOnly = true;
                    candidate.classList.add("solved");
                });
                await refreshFeedback();
                if (!inputs.some((candidate) => !candidate.readOnly)) {
                    window.location.reload();
                    return;
                }
                const next = nextInput && !nextInput.readOnly ? nextInput : inputs.find((candidate) => !candidate.readOnly);
                if (next) next.focus();
            } catch (error) {
                input.value = "";
                input.setCustomValidity(error.message);
                input.reportValidity();
                input.focus();
            } finally {
                delete input.dataset.submitting;
            }
        });
    });
})();
