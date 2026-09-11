async (page) => {
  const results = [];
  let mode = "unauthenticated";
  let calls = 0;
  const chatRequests = [];
  page.on("request", request => { if (request.url().includes("/api/") && request.url().includes("chat")) chatRequests.push(request.url()); });
  await page.unroute("**/api/v1/dashboard/overview");
  await page.route("**/api/v1/dashboard/overview", route => {
    calls++;
    if (mode === "unauthenticated") return route.fulfill({status: 401, json: {error:"fixture"}});
    if (mode === "error") return route.fulfill({status: 503, json: {error:"fixture"}});
    const usage = {calls: 0, inputTokensEstimated: 0, outputTokensEstimated: 0, totalTokensEstimated: 0};
    return route.fulfill({json: {user: {email:"preview@example.test", isAdmin:false}, devices:{paired:0,online:0}, workspaces:{total:0,active:0,sleeping:0,offline:0,recent:[]}, usage:{available:false,estimated:true,scope:"test-fixture",last24h:usage,last30d:usage,allTime:usage}}});
  });
  await page.goto("http://127.0.0.1:3100/dashboard?deviceId=old-chat-link&workspaceId=old-workspace");
  await page.getByText("Cần đăng nhập", {exact:true}).waitFor();
  results.push("401: sign-in feedback; old chat URLs render overview");
  mode = "error";
  await page.reload();
  await page.getByRole("button", {name:"Thử lại",exact:true}).waitFor();
  mode = "empty";
  await page.getByRole("button", {name:"Thử lại",exact:true}).click();
  await page.getByText("Không gian cho dự án đầu tiên.", {exact:true}).waitFor();
  if (await page.getByRole("region", {name:"Số liệu tài khoản"}).getByRole("link").filter({hasText:"Chưa có dữ liệu sử dụng"}).count() !== 2) throw new Error("Missing unavailable usage labels");
  results.push("503: retry recovers to empty state; unavailable telemetry is not shown as zero");
  const before = calls;
  await page.getByRole("button", {name:"Làm mới",exact:true}).click();
  await page.getByText("Không gian cho dự án đầu tiên.", {exact:true}).waitFor();
  if (calls !== before + 1) throw new Error("Refresh did not refetch overview");
  results.push("Refresh refetches the overview");
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:"output/playwright/dashboard-empty-mobile.png",fullPage:true});
  if (await page.locator("textarea").count() !== 0 || chatRequests.length) throw new Error("Web chat is still mounted");
  results.push("No chat composer or chat API requests");
  await page.getByRole("button",{name:"Mở điều hướng",exact:true}).click();
  await page.waitForFunction(() => document.querySelector("#dashboard-sidebar").contains(document.activeElement));
  await page.keyboard.press("Shift+Tab");
  if (!(await page.locator("#dashboard-sidebar").evaluate(sidebar => sidebar.contains(document.activeElement)))) throw new Error("Drawer focus escaped");
  await page.keyboard.press("Escape");
  if (!(await page.getByRole("button",{name:"Mở điều hướng",exact:true}).evaluate(button => button === document.activeElement))) throw new Error("Focus not restored");
  results.push("Mobile drawer traps keyboard focus and restores focus on Escape");
  await page.getByRole("link",{name:"CodeLocal.",exact:true}).click();
  await page.waitForURL("http://127.0.0.1:3100/");
  for (const width of [320,390,768,1024,1440]) {
    await page.setViewportSize({width,height:900});
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Landing overflows at ${width}`);
  }
  await page.setViewportSize({width:390,height:844});
  await page.emulateMedia({reducedMotion:"reduce"});
  await page.screenshot({path:"output/playwright/landing-mobile.png",fullPage:true});
  results.push("Landing has no horizontal overflow at 320, 390, 768, 1024, 1440px; reduced-motion renders");
  return results;
}
