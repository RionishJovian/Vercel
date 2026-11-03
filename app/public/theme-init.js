try{var t=localStorage.getItem("mm_theme");document.documentElement.dataset.theme=t==="day"||t==="night"?t:(matchMedia("(prefers-color-scheme: light)").matches?"day":"night")}catch(e){}
