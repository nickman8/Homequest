const $=s=>document.querySelector(s),frame=$('iframe');
const html=await(await fetch('/widget.html')).text();let state=null;
const palettes={light:{bg_color:'#ffffff',text_color:'#24382e',hint_color:'#738173',button_color:'#496d52',button_text_color:'#ffffff',secondary_bg_color:'#eef3e9',accent_text_color:'#557a4c'},dark:{bg_color:'#202a24',text_color:'#e7ede5',hint_color:'#a3b29f',button_color:'#a0c49a',button_text_color:'#1c2a1b',secondary_bg_color:'#303e33',accent_text_color:'#c5d6a9'},custom:{bg_color:'#f3eedc',text_color:'#3e4431',hint_color:'#828167',button_color:'#6a794f',button_text_color:'#ffffff',secondary_bg_color:'#e6e6cc',accent_text_color:'#8b7436'}};
function render(){
 if(!state)return;
 const theme=$('#theme').value,colors=palettes[theme],width=Number($('#width').value);frame.style.width=width+'px';$('.phone').style.width=(width+48)+'px';frame.hidden=false;$('#reply').hidden=true;
 const vars=Object.entries(colors).map(([k,v])=>'--tg-theme-'+k.replaceAll('_','-')+':'+v).join(';');
 // This bridge is ONLY in the development page; it is never in widgets/family.html.
 const boot=`var handlers={},pending={};window.TelegramWidget={chatData:${JSON.stringify(JSON.stringify(state)).replaceAll('<','\\u003c')},onEvent:function(n,f){handlers[n]=f},ready:function(){},request:function(data){return new Promise(function(resolve,reject){pending[data]={resolve:resolve,reject:reject};parent.postMessage({type:'request',data:data},'*')})}};addEventListener('message',function(e){if(e.source!==parent)return;var p=pending[e.data.action];if(p){if(e.data.error)p.reject({code:'FAILED'});else p.resolve({result:null});delete pending[e.data.action]}});new ResizeObserver(function(){parent.postMessage({type:'height',height:document.body.scrollHeight+16},'*')}).observe(document.documentElement);`;
 frame.srcdoc=`<!doctype html><meta charset="utf-8"><style>:root{${vars};color-scheme:${theme==='dark'?'dark':'light'}}body{margin:8px 12px;font:15px system-ui;background:var(--tg-theme-bg-color)}*{box-sizing:border-box}</style><script>${boot}</script>${html}`;
}
async function update(text='',action=''){
 const r=await fetch('/update',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({viewer:Number($('#viewer').value),text,action})});
 if(!r.ok)throw Error('handler');const result=await r.json();
 if(result.state){state=result.state;render()}else{state=null;frame.hidden=true;$('#reply').hidden=false;$('#reply').textContent=result.payload?.text||'Откройте дом';}
 return result;
}
window.addEventListener('message',async e=>{
 if(e.source!==frame.contentWindow)return;
 if(e.data.type==='height'){frame.style.height=Math.min(600,e.data.height)+'px';return}
 if(e.data.type!=='request'||typeof e.data.data!=='string')return;
 const source=e.source,action=e.data.data;try{await update('',action);source.postMessage({action},'*')}catch{source.postMessage({action,error:true},'*')}
});
$('#home').onclick=()=>update('/start');$('#viewer').onchange=()=>update('/start');$('#theme').onchange=render;$('#width').onchange=render;
$('#composer').onsubmit=async e=>{e.preventDefault();const input=$('#message');try{await update(input.value);input.value=''}catch{$('#reply').hidden=false;$('#reply').textContent='Не удалось обработать сообщение. Попробуйте снова.'}};
await update('/start');
