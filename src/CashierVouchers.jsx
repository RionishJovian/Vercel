import React,{useState} from 'react';
import {VoucherPayment} from './VoucherPayments.jsx';
export default function CashierVouchers({vouchers,act,busy}){
 const [id,setId]=useState('');const selected=vouchers.find(v=>v.id===id);
 return <section className="panel"><h2>Voucher counter payments</h2><p>Cashiers issue codes using Batch issue above, including batches of one. Refunds and code revocation require the owner or manager.</p><div className="tablewrap"><table><thead><tr><th>Voucher ending / package</th><th>Status</th><th>Counter payment</th></tr></thead><tbody>{vouchers.map(v=><tr key={v.id}><td>•••• {v.code_hint}<small>{v.policy.name}</small></td><td>{v.status}</td><td>{v.sale?<span>{v.sale.refunded_at?'Refunded':'Confirmed'} · {v.sale.amount} {v.sale.currency} · {v.sale.reference}</span>:<button className="button secondary" disabled={busy} onClick={()=>setId(v.id)}>Record payment</button>}</td></tr>)}</tbody></table></div>{selected&&!selected.sale&&<VoucherPayment key={selected.id} voucher={selected} act={act} busy={busy}/>}</section>
}
