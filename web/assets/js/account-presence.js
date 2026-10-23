/* Observed app presence only: no keystroke content, router state, or tracking IDs. */
window.MMAccountPresence = (() => {
  let started=false, inFlight=false, stopped=false, lastActivity=Date.now(), lastImmediate=0;
  const activity=()=>{lastActivity=Date.now();if(Date.now()-lastImmediate>30000){lastImmediate=Date.now();void beat();}};
  async function beat(){
    if(stopped || inFlight || !navigator.onLine)return;
    inFlight=true;
    try {await MM.post('/me/presence',{visible:document.visibilityState==='visible',idle_seconds:Math.min(86400,Math.max(0,Math.floor((Date.now()-lastActivity)/1000)))});}
    catch(error){if(error.status===401)stopped=true;/* Missing observations expire; never fabricate online. */}
    finally {inFlight=false;}
  }
  return {start(){
    if(started)return;started=true;
    for(const event of ['pointerdown','keydown','scroll','touchstart'])document.addEventListener(event,activity,{passive:true,capture:true});
    document.addEventListener('visibilitychange',()=>void beat());
    window.addEventListener('online',()=>void beat());
    window.setInterval(()=>void beat(),30000);
    void beat();
  },stop(){stopped=true;}};
})();
