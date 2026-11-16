var W=TelegramWidget,serial=0,page=0;
function feedback(text){document.getElementById('feedback').textContent=text}
function draw(){
 var s;try{s=JSON.parse(W.chatData||'{}')}catch(e){s={}}
 serial++;
 document.getElementById('title').textContent=s.title||'Дом';
 var title=s.title||'',labels=(s.buttons||[]).map(function(b){return b.label}).join(' '),icon='house';
 if(/балл|вклад|Рейтинг|Достижен|Premium/.test(title))icon='trophy';
 else if(/фото|Отчёт|подтвержден|Готово/.test(title))icon='circle-check';
 else if(/Участник|Исполнитель|Проверяющий/.test(title))icon='users';
 else if(/Помощник|Черновик/.test(title))icon='mic';
 else if(/Настрой|Управление/.test(title))icon='settings';
 else if(/Напомин|расписан/.test(title))icon='calendar-days';
 else if(/Забронировать|бронь|Запросить|Заявки/.test(labels)||/Подар|Наград/.test(title))icon='gift';
 else if(/Выполнено/.test(labels)||title==='Сегодня')icon='circle-check';
 document.querySelectorAll('.mark svg').forEach(function(node){node.style.display=node.id==='icon-'+icon?'block':'none'});
 document.querySelector('.panel').className=title.length>35?'panel compact':'panel';
 var root=document.getElementById('buttons');root.textContent='';
 var narrow=window.innerWidth<240,buttons=s.buttons||[],size=title.length>35?(narrow?1:2):(narrow?3:Math.max(1,buttons.length));
 var text=Array.from(s.text||'Откройте /start, чтобы вернуться домой'),limit=narrow?60:180,textPages=Math.ceil(text.length/limit),pages=Math.max(textPages,Math.ceil(buttons.length/size));
 if(page>=pages)page=0;
 var textPage=Math.min(page,textPages-1);
 document.getElementById('text').textContent=(textPage?'…':'')+text.slice(textPage*limit,(textPage+1)*limit).join('')+(textPage+1<textPages?'…':'');
 buttons.slice(page*size,(page+1)*size).forEach(function(item){
  var b=document.createElement('button');b.textContent=item.label;
  b.onclick=function(){
   var version=serial;b.disabled=true;feedback('Отправляю…');
   function finish(text){b.disabled=false;if(serial===version)feedback(text)}
   try{W.request(item.action).then(function(){finish('Запрос принят. Ответ появится в карточке или сообщении.')},function(e){
    var errors={RATE_LIMITED:'Слишком быстро. Подождите немного и повторите.',TIMEOUT:'Ответ задержался. Можно повторить — действие не задвоится.',NOT_ALLOWED:'Откройте свежую карточку через /start.',BUSY:'Дождитесь ответа на предыдущее действие.'};
    finish(errors[e.code]||'Не удалось связаться. Попробуйте снова.')
   })}catch(e){finish('Не удалось отправить. Попробуйте снова.')}
  };root.appendChild(b)
 });
 if(pages>1){var next=document.createElement('button');next.textContent=(page+1<pages?(textPages>1?'Далее':'Ещё действия'):'К началу')+' · '+((page+1)%pages+1)+'/'+pages;next.onclick=function(){page=(page+1)%pages;draw()};root.appendChild(next)}
 feedback('Вместе заботимся о доме');
}
W.onEvent('chatDataChanged',function(){page=0;draw()});draw();W.ready();
