#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const manifest = resolve(root, "modules/HarnessKit/Cargo.toml");
const output = resolve(process.argv[2] || "build/compliance/harnesskit.spdx.json");
const metadata = JSON.parse(
  execFileSync(
    "cargo",
    [
      "metadata",
      "--format-version",
      "1",
      "--locked",
      "--manifest-path",
      manifest,
    ],
    { encoding: "utf8", maxBuffer: 32 * 1024 * 1024 },
  ),
);

const packages = metadata.packages
  .map((pkg, index) => {
    const downloadLocation =
      pkg.source?.replace(/^registry\+/, "") ||
      pkg.repository ||
      "NOASSERTION";
    return {
      name: pkg.name,
      SPDXID: `SPDXRef-Package-${index}-${pkg.name.replace(/[^A-Za-z0-9.-]/g, "-")}`,
      versionInfo: pkg.version,
      downloadLocation,
      filesAnalyzed: false,
      licenseConcluded: "NOASSERTION",
      licenseDeclared: pkg.license || "NOASSERTION",
      copyrightText: "NOASSERTION",
      ...(pkg.homepage
        ? {
            externalRefs: [
              {
                referenceCategory: "OTHER",
                referenceType: "website",
                referenceLocator: pkg.homepage,
              },
            ],
          }
        : {}),
    };
  })
  .sort((a, b) =>
    `${a.name}@${a.versionInfo}`.localeCompare(`${b.name}@${b.versionInfo}`),
  );

const sourceDate = process.env.SOURCE_DATE_EPOCH;
const created = sourceDate
  ? new Date(Number(sourceDate) * 1000).toISOString()
  : new Date().toISOString();
const namespaceSeed = process.env.GITHUB_SHA || "local";
const document = {
  spdxVersion: "SPDX-2.3",
  dataLicense: "CC0-1.0",
  SPDXID: "SPDXRef-DOCUMENT",
  name: "1agents-controlled-HarnessKit",
  documentNamespace: `https://github.com/scottzx/1Agents/sbom/harnesskit/${namespaceSeed}`,
  creationInfo: {
    created,
    creators: ["Tool: scripts/generate-harnesskit-sbom.mjs"],
  },
  documentDescribes: packages.map((pkg) => pkg.SPDXID),
  packages,
};

mkdirSync(dirname(output), { recursive: true });
writeFileSync(output, `${JSON.stringify(document, null, 2)}\n`);
console.log(`HarnessKit SPDX SBOM: ${output} (${packages.length} packages)`);
