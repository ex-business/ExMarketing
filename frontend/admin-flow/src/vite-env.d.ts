/// <reference types="vite/client" />

declare module 'luaparse' {
  export type LuaParseOptions = {
    comments?: boolean;
    locations?: boolean;
    luaVersion?: string;
    ranges?: boolean;
    scope?: boolean;
  };

  export function parse(input: string, options?: LuaParseOptions): unknown;
}
