
export function ws_protocol() {
	return (
      [1e7]+-1e3+-4e3+-8e3+-1e11).replace(/[018]/g,
      c => (c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> c / 4).toString(16)
    );
}

export function object_get(obj, k) { 
	try {
		return obj[k]
	} catch(x) {
		return undefined
	}
};
export function object_set(obj, k, v) {
	try { obj[k] = v } catch {}
};

export async function convert_body_inner(body) {
	let req = new Request("", { method: "POST", duplex: "half", body });
	let type = req.headers.get("content-type");
	return [new Uint8Array(await req.arrayBuffer()), type];
}

export async function convert_streaming_body_inner(body) {
	try {
		let req = new Request("", { method: "POST", body });
		let type = req.headers.get("content-type");
		return [false, new Uint8Array(await req.arrayBuffer()), type];
	} catch(x) {
		let req = new Request("", { method: "POST", duplex: "half", body });
		let type = req.headers.get("content-type");
		return [true, req.body, type];
	}
}

export function entries_of_object_inner(obj) {
	return Object.entries(obj).map(x => x.map(String));
}

export function define_property(obj, k, v) {
	Object.defineProperty(obj, k, { value: v, writable: false });
}

export function ws_key() {
	let key = new Uint8Array(16);
	crypto.getRandomValues(key);
	return btoa(String.fromCharCode.apply(null, key));
}

export function from_entries(entries){
    var ret = {};
    for(var i = 0; i < entries.length; i++) ret[entries[i][0]] = entries[i][1];
    return ret;
}
