import { test, expect } from "./fixtures.ts";

// The preview fixtures: internal/storage/fixtures/preview.json.
const videoTitles = [
  "Fixture: Postgres indexes in fifteen minutes",
  "Fixture: Context cancellation in Go",
  "Fixture: Sourdough starter from scratch",
  "Fixture: Vortrag über Wärmepumpen",
];

test("the library lists the fixture videos and no podcast episodes", async ({ page }) => {
  await page.goto("/");
  for (const title of videoTitles) {
    await expect(page.getByRole("heading", { name: title, level: 3 })).toBeVisible();
  }
  await expect(page.getByRole("heading", { name: /Fixture Weekly/ })).toHaveCount(0);
});

test("opening a video shows its summary", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: /Context cancellation in Go/ }).click();
  await expect(page).toHaveURL(/\/video\/[0-9a-f-]{36}$/);
  await expect(
    page.getByRole("heading", { name: "Fixture: Context cancellation in Go", level: 1 }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "Overview", level: 2 })).toBeVisible();
});

test("search narrows the library to the matching video", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("textbox", { name: "Search summaries" }).fill("sourdough");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\?q=sourdough$/);
  await expect(
    page.getByRole("heading", { name: "Fixture: Sourdough starter from scratch" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: videoTitles[0] })).toHaveCount(0);
});

for (const [path, heading] of [
  ["/inbox", "Inbox"],
  ["/channels", "Channels & shows"],
  ["/stats", "Stats"],
  ["/settings", "Settings"],
] as const) {
  test(`${path} renders`, async ({ page }) => {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: heading, level: 1 })).toBeVisible();
  });
}

test("without application login the nav offers no sign-out", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("navigation").getByRole("link", { name: "Settings" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign out" })).toHaveCount(0);
});
