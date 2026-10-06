export function budgetSummary(b){
 if(!b.enabled||b.used_gb===null||!b.reading_date||!b.cycle_start||!b.cycle_end)return null;
 const day=s=>Date.parse(s+'T00:00:00Z')/86400000;const elapsed=day(b.reading_date)-day(b.cycle_start)+1,total=day(b.cycle_end)-day(b.cycle_start),remainingDays=Math.max(0,total-elapsed);
 if(!Number.isFinite(elapsed)||!Number.isFinite(total)||!Number.isFinite(b.used_gb)||!Number.isFinite(b.limit_gb)||b.limit_gb<=0||b.used_gb<0||elapsed<=0||elapsed>total||total<=0)return null;
 const remaining=Math.max(0,b.limit_gb-b.used_gb),ratio=b.used_gb/b.limit_gb*100,projected=elapsed>0?b.used_gb/elapsed*total:null;
 return {remaining,ratio,projected,remainingDays,dailyAllowance:remainingDays>0?remaining/remainingDays:null,state:ratio>=100?'Budget exceeded':ratio>=b.warning_percent?'Warning threshold reached':'Below warning threshold'};
}
