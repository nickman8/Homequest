// Browser development bridge E2E. This is NOT a Telegram compatibility test.
import {spawn} from 'node:child_process';
import {createRequire} from 'node:module';
import path from 'node:path';
import fs from 'node:fs';
import assert from 'node:assert/strict';
const require=createRequire(path.join(process.env.WIDGET_SANDBOX!,'package.json'));
const {chromium}=require('@playwright/test'),port='4174',url=`http://127.0.0.1:${port}`;
const server=spawn('node',['scripts/preview-server.mjs'],{env:{...process.env,HOMEQUEST_PREVIEW_PORT:port},stdio:'pipe'});
let browser;
try{
 for(let n=0;n<50;n++){try{if((await fetch(url)).ok)break}catch{}await new Promise(r=>setTimeout(r,100))}
 browser=await chromium.launch();const page=await browser.newPage({viewport:{width:1160,height:1040}});
 const errors:string[]=[];page.on('pageerror',(e:any)=>errors.push(e.message));
 await page.goto(url);const frame=page.frameLocator('iframe');
 const click=async(label:string)=>{await frame.getByRole('button',{name:label,exact:true}).click()};
 const send=async(text:string)=>{await page.locator('#message').fill(text);await page.locator('#composer button').click();await page.waitForFunction(()=>!(document.querySelector('#message') as HTMLInputElement).value)};
 await click('Создать дом');await page.locator('#reply').waitFor({state:'visible'});
 await send('Тёплый дом');await send('Europe/Berlin');
 await click('Настройки');await click('Пригласить');await page.locator('#reply').waitFor({state:'visible'});
 const token=(await page.locator('#reply').textContent()).split('join_')[1].trim();assert.ok(token);
 await page.locator('#viewer').selectOption('900002');await frame.getByRole('button',{name:'Создать дом',exact:true}).waitFor();
 await send('/start join_'+token);await frame.getByRole('button',{name:'Сегодня',exact:true}).waitFor();
 await click('Добавить задание');await send('Вынести мусор');await send('10');
 await frame.getByRole('button',{name:'Выполнено',exact:true}).waitFor();
 fs.mkdirSync('docs/screenshots',{recursive:true});
 for(const theme of ['light','dark']){await page.locator('#theme').selectOption(theme);await frame.getByRole('button',{name:'Выполнено',exact:true}).waitFor();await page.screenshot({path:`docs/screenshots/preview-${theme}.png`,fullPage:true})}
 await click('Выполнено');await frame.getByText('Пока нет доступных заданий.',{exact:false}).waitFor();await click('Дом');await click('Мои баллы');
 await frame.getByText('Баланс: 10',{exact:false}).waitFor();
 assert.deepEqual(errors,[]);
 console.log('PASS browser bridge → handlers → SQLite: family/join/task/completion/balance; light/dark screenshots');
}finally{await browser?.close();server.kill('SIGTERM');await new Promise(r=>server.once('exit',r))}
