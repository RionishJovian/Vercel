"""Private normalized receipt images; no public file paths or original filenames."""
import hashlib,time
from io import BytesIO
from fastapi import Depends,HTTPException,Body,Header,Response
from sqlalchemy import select,update,func
from PIL import Image,ImageOps,UnidentifiedImageError
from .models import ReceiptImage,Order
MAX_INPUT=5*1024*1024
MAX_STORED=8*1024*1024
MAX_PIXELS=8_000_000

def normalize(raw):
 if not raw or len(raw)>MAX_INPUT:raise HTTPException(413,'Choose a JPEG or PNG up to 5 MiB')
 try:
  with Image.open(BytesIO(raw),formats=['JPEG','PNG']) as im:
   if im.width*im.height>MAX_PIXELS or getattr(im,'n_frames',1)!=1:raise HTTPException(422,'Use a single-frame image with at most 8 million pixels')
   im.load();im=ImageOps.exif_transpose(im)
   # New pixel-only image excludes embedded metadata and appended input content.
   converted=im.convert('RGBA' if 'A' in im.getbands() else 'RGB')
   clean=Image.new(converted.mode,converted.size);clean.paste(converted)
   out=BytesIO();clean.save(out,format='PNG');value=out.getvalue()
   if len(value)>MAX_STORED:raise HTTPException(413,'Image is too large after decoding. Export a smaller receipt image.')
   return value,clean.width,clean.height
 except (UnidentifiedImageError,OSError,ValueError,Image.DecompressionBombError) as e:raise HTTPException(422,'The receipt must be a valid JPEG or PNG image') from e

def receipt_json(r):return {'id':r.id,'created':r.created,'width':r.width,'height':r.height,'bytes':len(r.image),'original_sha256':r.original_hash,'representation':'normalized_png'}
def receipt_metadata(s,order_id):
 rows=s.execute(select(ReceiptImage.id,ReceiptImage.created,ReceiptImage.width,ReceiptImage.height,func.length(ReceiptImage.image).label('bytes'),ReceiptImage.original_hash.label('original_sha256')).where(ReceiptImage.order_id==order_id).order_by(ReceiptImage.created,ReceiptImage.id)).mappings()
 return [{**r,'representation':'normalized_png'} for r in rows]
def register_receipts(app,current,db,order_for,log):
 @app.get('/v1/orders/{id}/receipts')
 def list_receipts(id:str,u=Depends(current),s=Depends(db)):
  order_for(s,u,id)
  return receipt_metadata(s,id)
 @app.post('/v1/orders/{id}/receipts',status_code=201)
 def upload(id:str,data:bytes=Body(media_type='application/octet-stream'),request_key:str=Header(alias='X-Upload-Key',min_length=16,max_length=80,pattern=r'^[A-Za-z0-9-]+$'),u=Depends(current),s=Depends(db)):
  if u.role!='guest':raise HTTPException(403,'Only the purchasing guest can attach receipts')
  o=order_for(s,u,id);digest=hashlib.sha256(data).hexdigest()
  # Serialize attachment count, review transitions and concurrent upload retries on the order row.
  if not s.execute(update(Order).where(Order.id==id).values(receipt_revision=Order.receipt_revision)).rowcount:raise HTTPException(404,'Order not found')
  s.refresh(o)
  existing=s.scalar(select(ReceiptImage).where(ReceiptImage.order_id==id,ReceiptImage.request_key==request_key))
  if existing:
   if existing.original_hash!=digest:raise HTTPException(409,'Upload key reused for another image')
   return {**receipt_json(existing),'replayed':True}
  if o.status not in ['pending','needs_evidence']:raise HTTPException(409,'This payment decision is closed; additional receipts cannot be attached')
  if s.scalar(select(func.count()).select_from(ReceiptImage).where(ReceiptImage.order_id==id))>=3:raise HTTPException(409,'This order already has three receipt images')
  image,width,height=normalize(data)
  r=ReceiptImage(order_id=id,uploaded_by=u.id,request_key=request_key,original_hash=digest,image=image,width=width,height=height,created=int(time.time()));s.add(r);o.receipt_revision+=1;s.flush();log(s,u,'receipt_attached',r.id);s.commit();return receipt_json(r)
 @app.get('/v1/orders/{id}/receipts/{receipt_id}')
 def view(id:str,receipt_id:str,u=Depends(current),s=Depends(db)):
  order_for(s,u,id);r=s.get(ReceiptImage,receipt_id)
  if not r or r.order_id!=id:raise HTTPException(404,'Receipt not found')
  log(s,u,'receipt_viewed',r.id);s.commit()
  return Response(r.image,media_type='image/png',headers={'Content-Disposition':'inline; filename="receipt.png"','Content-Security-Policy':"default-src 'none'; sandbox",'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'})
