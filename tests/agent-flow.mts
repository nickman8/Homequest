import {Flow} from './flow-driver.mts';
import assert from 'node:assert/strict';
export class AgentFlow extends Flow {
 async request(operation:string,text:string,fields:any={}){
  await this.raw({tick:true,agent_fixture:{operation,...fields}});
  await this.send(text);
  if(this.last.state?.buttons.some((b:any)=>b.label==='Понятно, продолжить'))await this.click('Понятно, продолжить');
  await this.inject({tick:true,process_ai:true});
 }
 async preview(){
  assert.equal(this.last.state?.title,'Проверьте изменения');
  let summary='';
  for(let n=0;n<100;n++){
   summary+=this.last.state.text+'\n';
   if(this.last.state.buttons.some((b:any)=>b.label==='Подтвердить план'))return summary;
   await this.click('Следующая часть');
  }
  throw Error('Preview pagination did not finish');
 }
 async apply(){const summary=await this.preview();await this.click('Подтвердить план');assert.equal(this.last.state.title,'План применён');return summary;}
 denied(){assert.match(this.last.payload.text,/безопасно/);}
 async reports(){await this.home();await this.click('Настройки');await this.click('Отчёты');}
}
