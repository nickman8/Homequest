import fs from 'node:fs';
function tokens(name){return Object.fromEntries([...fs.readFileSync(new URL(import.meta.resolve(name)), 'utf8').matchAll(/(--[\w-]+)\s*:\s*([^;}]+)/g)].map(m=>[m[1],m[2]]))}
const sizes=tokens('open-props/sizes'),borders=tokens('open-props/borders');
import {deflateSync} from 'node:zlib';
const icon=fs.readFileSync('node_modules/lucide-static/icons/house.svg','utf8').replace(/<\?xml[^>]*>/g,'');
const html=`<style>.panel{padding:${sizes['--size-3']};border-radius:${borders['--radius-3']};background:var(--tg-theme-secondary-bg-color)}button{min-height:44px;width:100%;margin-top:${sizes['--size-2']}}svg{width:24px;height:24px}</style><div class="panel">${icon}<h3>Дом</h3><p id="status">Проверка подключения</p><button id="refresh">Обновить</button></div><script>function draw(){var s;try{s=JSON.parse(TelegramWidget.chatData||'{}')}catch(e){s={}}document.getElementById('status').textContent=s.status||'Откройте новую карточку';document.getElementById('refresh').disabled=!s.action}document.getElementById('refresh').onclick=function(){var s=JSON.parse(TelegramWidget.chatData||'{}');TelegramWidget.request(s.action).catch(function(){document.getElementById('status').textContent='Не удалось обновить. Попробуйте ещё раз'})};TelegramWidget.onEvent('chatDataChanged',draw);draw();TelegramWidget.ready();</script>`;
if(Buffer.byteLength(html)>32768||deflateSync(html,{level:9}).length>3500)throw Error('Widget budget exceeded');
fs.writeFileSync('widgets/home.html',html);
const names=['house','circle-check','gift','trophy','calendar-days','mic','users','settings'];
const icons=names.map(name=>fs.readFileSync('node_modules/lucide-static/icons/'+name+'.svg','utf8').replace(/<\?xml[^>]*>/g,'').replace(/<!--[^]*?-->/g,'').replace('<svg','<svg id="icon-'+name+'"')).join('');
const css=fs.readFileSync('widgets/src/card.css','utf8').replaceAll('__SPACE__',sizes['--size-3']).replaceAll('__GAP__',sizes['--size-2']).replaceAll('__RADIUS__',borders['--radius-3']);
const js=fs.readFileSync('widgets/src/card.js','utf8');
const familyHTML=`<style>${css}</style><div class="panel"><div class="brand"><div class="mark">${icons}</div><span>HOMEQUEST · НАШ ДОМ</span></div><h3 id="title">Дом</h3><p id="text">Загрузка</p><div id="buttons"></div><p id="feedback"></p></div><script>${js}</script>`;
if(Buffer.byteLength(familyHTML)>32768||deflateSync(familyHTML,{level:9}).length>3500)throw Error('Family widget budget');
fs.writeFileSync('widgets/family.html',familyHTML);
console.log(JSON.stringify({htmlBytes:Buffer.byteLength(familyHTML),zlib9Bytes:deflateSync(familyHTML,{level:9}).length}));
