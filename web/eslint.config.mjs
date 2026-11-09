import next from "eslint-config-next";

const config = [{ ignores: [".next/**", "next-env.d.ts", "public/sw.js"] }, ...next];

export default config;
