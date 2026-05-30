import * as esbuild from "esbuild";
import { readdirSync, copyFileSync, statSync } from "fs";
import { join, relative, resolve } from "path";
import { mkdirSync, readFileSync, rmSync, watch, writeFileSync } from "fs";

const outdir = "dist"
const staticFileDirs = ["src"];
const staticFilePattern = /\.(html|css|json)$/;

const config = {
  entryPoints: ["src/index.ts"],
  outdir: outdir,
  bundle: true,
  target: ["es2024","firefox150"],
  format: "esm",
  sourcemap: true,
  // minify: true,
};

function findFiles(dir, matches, result) {
  for (const file of readdirSync(dir)) {
    const filepath = join(dir, file);
    const stat = statSync(filepath);
    if (stat.isDirectory()) {
      findFiles(filepath, result);
    } else if (matches.test(file)) {
      result.push(filepath);
    }
  }
}

function copyStaticFiles() {
  for (const fromDir of staticFileDirs) {
    const files = [];
    findFiles(fromDir, staticFilePattern, files);
    for (const file of files) {
      const dest = join(outdir, relative(fromDir, file));
      copyFileSync(file, dest);
    }
  }
}

// ============================================================================

console.log(`Clearing ${outdir}...`);
rmSync(outdir, { recursive: true, force: true });
mkdirSync(outdir, { recursive: true });
console.log("Copying static files...");
copyStaticFiles();

console.log("Running esbuild...");
const ctx = await esbuild.context(config);

if (process.argv.includes("--watch")) {
  await ctx.rebuild();
  await ctx.watch();
  for (const dir of staticFileDirs) {
    watch(dir, { persistent: false, recursive: true }, (eventType, filename) => {
      if (staticFilePattern.test(filename)) {
        console.log(`Copying static files due to change in: ${filename}`);
        copyStaticFiles();
      }
    });
  }
} else {
  await ctx.rebuild();
  await ctx.dispose();
  console.log("Built successfully.");
}