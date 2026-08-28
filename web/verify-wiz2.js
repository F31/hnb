const { chromium } = require('./node_modules/.pnpm/playwright@1.62.0/node_modules/playwright');
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errs = [];
  page.on('pageerror', e => errs.push(e.message));
  page.on('console', m => { if (m.type()==='error') errs.push(m.text()); });
  await page.goto('http://localhost:3000/login', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1200);
  await page.fill('input[placeholder="请输入用户名"]', 'admin');
  await page.fill('input[placeholder="请输入密码"]', 'Dev@admin123');
  await page.locator('button:has-text("登")').click();
  await page.waitForTimeout(5000);
  await page.locator('text=资源').first().click();
  await page.waitForTimeout(4000);
  await page.locator('button:has-text("接入集群")').first().click();
  await page.waitForTimeout(1500);
  const b = await page.locator('.modal-card').innerText();
  const fields = ['集群名称','集群架构','集群描述','集群插件','控制节点调度','IPV6 双栈','容器网络','管理网 VIP','管理网卡名','集群网 VIP','集群网卡名','POD 网段 CIDR','Service 网段 CIDR','Join 网段 CIDR','高级设置'];
  for (const f of fields) console.log(f, ':', b.includes(f) ? 'PASS' : 'FAIL');
  // architecture select options
  const archOpts = await page.locator('.form-grid select').first().locator('option').allTextContents();
  console.log('architecture options:', JSON.stringify(archOpts));
  // checkboxes present
  console.log('checkbox count:', await page.locator('.form-grid input[type="checkbox"]').count());
  // advanced section
  await page.locator('.advanced summary').click();
  await page.waitForTimeout(500);
  const adv = await page.locator('.advanced').innerText();
  console.log('advanced fields:', ['自定义证书 SAN','集群域名','监控告警配置','通知方式','告警通知间隔','已回复告警是否通知','联系人'].every(f => adv.includes(f)) ? 'PASS' : 'FAIL');
  console.log('ERRORS:', errs.length ? JSON.stringify(errs) : 'none');
  await browser.close();
})();
