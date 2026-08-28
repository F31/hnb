const { chromium } = require('./node_modules/.pnpm/playwright@1.62.0/node_modules/playwright');
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  let posted = null;
  page.on('request', req => { if (req.url().includes('/runtime-intents') && req.method()==='POST') posted = req.postData(); });
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
  // fill name + a few fields
  await page.fill('.field-name input', 'wiz-test-cluster');
  await page.locator('.form-grid label').filter({ hasText: '集群描述' }).locator('textarea').fill('desc here');
  // select a plugin
  await page.locator('.checkbox-field input[type="checkbox"]').first().check();
  // toggle switches
  await page.locator('label.switch-field', { hasText: '控制节点调度' }).locator('input').check();
  // advanced: cluster domain + SAN
  await page.locator('.advanced summary').click();
  await page.waitForTimeout(300);
  await page.locator('.advanced input[placeholder*="cluster.local"]').fill('my.domain');
  await page.locator('.advanced textarea').first().fill('k8s.local\n10.0.0.1');
  // next
  await page.locator('button:has-text("下一步")').click();
  await page.waitForTimeout(500);
  const conf = await page.locator('.confirm-list').innerText();
  console.log('confirm shows:', conf.includes('wiz-test-cluster') && conf.includes('AARCH64') === false ? 'PASS' : 'PARTIAL');
  // submit
  await page.locator('footer button:has-text("提交")').click();
  await page.waitForTimeout(2500);
  if (posted) {
    const p = JSON.parse(posted);
    console.log('payload kind:', p.kind);
    const params = p.spec.parameters || {};
    const checks = ['architecture','description','plugins','controlPlaneSchedulingEnabled','clusterDomain','customCertSANs','podCidr'];
    for (const k of checks) console.log('param', k, ':', k in params ? 'PASS' : 'FAIL');
    console.log('params count:', Object.keys(params).length);
    console.log('architecture value:', params.architecture, '| plugins:', params.plugins, '| domain:', params.clusterDomain);
  } else {
    console.log('no POST captured (maybe error)');
  }
  await browser.close();
})();
