import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://dougmaitelli.github.io",
  base: "/CertVault",
  srcDir: ".",
  publicDir: "../assets",
  outDir: "./dist",
  integrations: [
    starlight({
      title: "CertVault",
      description: "Centrally managed TLS certificates for homelabs.",
      logo: {
        src: "./assets/logo-mark.svg",
        alt: "CertVault",
      },
      favicon: "/logo.svg",
      customCss: ["./styles/docs.css"],
      markdown: {
        processedDirs: ["."],
      },
      components: {
        Hero: "./components/Hero.astro",
      },
      editLink: {
        baseUrl: "https://github.com/dougmaitelli/CertVault/edit/master/docs/",
      },
      lastUpdated: true,
      credits: true,
      social: [
        {
          icon: "github",
          label: "CertVault on GitHub",
          href: "https://github.com/dougmaitelli/CertVault",
        },
      ],
      sidebar: [
        { label: "Overview", slug: "" },
        {
          label: "Start here",
          items: [
            { label: "Getting started", slug: "getting-started" },
            { label: "Configuration", slug: "configuration" },
            { label: "Authentication", slug: "authentication" },
            { label: "Security", slug: "security" },
            { label: "Operations", slug: "operations" },
          ],
        },
        {
          label: "Integration",
          items: [
            { label: "Client access", slug: "client-access" },
            { label: "API reference", slug: "api" },
          ],
        },
      ],
    }),
  ],
});
