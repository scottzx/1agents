// Polyfill for TextEncoder and TextDecoder in WeChat mini-program environments

if (typeof TextEncoder === 'undefined') {
  const TextEncoderPolyfill = class TextEncoder {
    encode(str: string): Uint8Array {
      const arr: number[] = [];
      for (let i = 0; i < str.length; i++) {
        let code = str.charCodeAt(i);
        if (code < 0x80) {
          arr.push(code);
        } else if (code < 0x800) {
          arr.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
        } else if (code < 0xd800 || code >= 0xe000) {
          arr.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
        } else {
          i++;
          code = 0x10000 + (((code & 0x3ff) << 10) | (str.charCodeAt(i) & 0x3ff));
          arr.push(
            0xf0 | (code >> 18),
            0x80 | ((code >> 12) & 0x3f),
            0x80 | ((code >> 6) & 0x3f),
            0x80 | (code & 0x3f)
          );
        }
      }
      return new Uint8Array(arr);
    }
  };
  (global as any).TextEncoder = TextEncoderPolyfill;
}

if (typeof TextDecoder === 'undefined') {
  const TextDecoderPolyfill = class TextDecoder {
    decode(arr: Uint8Array): string {
      let str = '';
      for (let i = 0; i < arr.length; i++) {
        let value = arr[i];
        if (value < 0x80) {
          str += String.fromCharCode(value);
        } else if (value > 0xbf && value < 0xe0) {
          str += String.fromCharCode(((value & 0x1f) << 6) | (arr[++i] & 0x3f));
        } else if (value > 0xdf && value < 0xf0) {
          str += String.fromCharCode(
            ((value & 0x0f) << 12) | ((arr[++i] & 0x3f) << 6) | (arr[++i] & 0x3f)
          );
        } else {
          const code =
            ((value & 0x07) << 18) |
            ((arr[++i] & 0x3f) << 12) |
            ((arr[++i] & 0x3f) << 6) |
            (arr[++i] & 0x3f);
          const ch = code - 0x10000;
          str += String.fromCharCode(0xd800 | (ch >> 10), 0xdc00 | (ch & 0x3ff));
        }
      }
      return str;
    }
  };
  (global as any).TextDecoder = TextDecoderPolyfill;
}
