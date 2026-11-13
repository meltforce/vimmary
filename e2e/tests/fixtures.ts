import { test as base, expect } from "@playwright/test";

// Every test fails on an uncaught exception in the page. Console messages
// about failed resource loads do not count: the fixture thumbnails point at
// YouTube IDs that do not exist.
export const test = base.extend<{ pageErrors: Error[] }>({
  pageErrors: [
    async ({ page }, use) => {
      const errors: Error[] = [];
      page.on("pageerror", (err) => errors.push(err));
      await use(errors);
      expect(errors.map((e) => e.message)).toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };
