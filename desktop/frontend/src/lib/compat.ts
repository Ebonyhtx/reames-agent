type ObjectConstructorWithHasOwn = ObjectConstructor & {
  hasOwn?: (value: object, property: PropertyKey) => boolean;
};

// Safari 15.3 and matching older macOS WebKit shells predate Object.hasOwn.
// react-markdown calls it while validating props, so install the tiny ES2022
// primitive before React imports the markdown chunk.
export function installObjectHasOwnPolyfill(target: ObjectConstructorWithHasOwn = Object): void {
  if (typeof target.hasOwn === "function") return;
  Object.defineProperty(target, "hasOwn", {
    configurable: true,
    writable: true,
    value(value: object, property: PropertyKey) {
      return Object.prototype.hasOwnProperty.call(value, property);
    },
  });
}

installObjectHasOwnPolyfill();
