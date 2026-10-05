/**
 * Vitest setup: jsdom lacks HTMLDialogElement.showModal/close. This
 * minimal polyfill mirrors the browser contract the app relies on
 * (open attribute, async "close" event). Dialog.test.tsx also covers the
 * app's own fallback for engines without showModal.
 */
const proto = HTMLDialogElement.prototype as HTMLDialogElement & { __lpPolyfill?: boolean };

if (typeof proto.showModal !== "function") {
  Object.assign(proto, {
    __lpPolyfill: true,
    showModal(this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
    close(this: HTMLDialogElement, returnValue?: string) {
      if (!this.hasAttribute("open")) return;
      this.removeAttribute("open");
      if (returnValue !== undefined) this.returnValue = returnValue;
      this.dispatchEvent(new Event("close"));
    },
  });
}
