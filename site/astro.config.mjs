import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';

// Served from the custom domain reames-agent.io at the site root.
export default defineConfig({
  site: 'https://reames-agent.io',
  build: { assets: 'static' },
  integrations: [sitemap()],
});
