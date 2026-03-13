import path from "node:path";
import fs from "node:fs/promises";

const playwrightModulePath =
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright";
const { chromium } = await import(playwrightModulePath);

const baseURL = process.env.README_BASE_URL || "http://127.0.0.1:18080";
const adminPassword = process.env.README_WEB_PASSWORD || "demo-admin-password";
const screenshotDir = process.env.README_SCREENSHOT_DIR || path.resolve("docs/images");
const chromiumPath = process.env.CHROMIUM_PATH || "/usr/bin/chromium";

async function login(page) {
  await page.goto(`${baseURL}/admin/`, { waitUntil: "networkidle" });
  await page.getByPlaceholder("请输入 WEB_PASSWORD").fill(adminPassword);
  await page.getByRole("button", { name: "进入管理台" }).click();
  await page.getByText("机器人配置列表").waitFor();
}

await fs.mkdir(screenshotDir, { recursive: true });

const browser = await chromium.launch({
  headless: true,
  executablePath: chromiumPath,
});

try {
  const context = await browser.newContext({
    viewport: { width: 1520, height: 1180 },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  await page.goto(`${baseURL}/admin/`, { waitUntil: "networkidle" });
  await page.screenshot({
    path: path.join(screenshotDir, "readme-admin-login.png"),
    fullPage: true,
  });

  await login(page);
  await page.screenshot({
    path: path.join(screenshotDir, "readme-admin-config.png"),
    fullPage: true,
  });

  const callbackHeading = page.getByText("接收消息服务器配置");
  await callbackHeading.scrollIntoViewIfNeeded();
  await page.screenshot({
    path: path.join(screenshotDir, "readme-admin-callback.png"),
    fullPage: false,
  });

  await page.goto(`${baseURL}/admin/tools`, { waitUntil: "networkidle" });
  await page.getByText("接口调试使用的配置").waitFor();
  await page.screenshot({
    path: path.join(screenshotDir, "readme-admin-tools.png"),
    fullPage: false,
  });

  await context.close();
} finally {
  await browser.close();
}
