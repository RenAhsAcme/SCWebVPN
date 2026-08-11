var YUVCanvas=function(){"use strict";var O={exports:{}},Y={exports:{}};(function(){function p(c,n){throw new Error("abstract")}p.prototype.drawFrame=function(c){throw new Error("abstract")},p.prototype.clear=function(){throw new Error("abstract")},Y.exports=p})();var k={exports:{}},H={exports:{}},$={exports:{}};(function(){/**
 * Convert a ratio into a bit-shift count; for instance a ratio of 2
 * becomes a bit-shift of 1, while a ratio of 1 is a bit-shift of 0.
 *
 * @author Brion Vibber <brion@pobox.com>
 * @copyright 2016
 * @license MIT-style
 *
 * @param {number} ratio - the integer ratio to convert.
 * @returns {number} - number of bits to shift to multiply/divide by the ratio.
 * @throws exception if given a non-power-of-two
 */function p(c){for(var n=0,a=c>>1;a!=0;)a=a>>1,n++;if(c!==1<<n)throw"chroma plane dimensions must be power of 2 ratio to luma plane dimensions; got "+c;return n}$.exports=p})(),function(){var p=$.exports;/**
 * Basic YCbCr->RGB conversion
 *
 * @author Brion Vibber <brion@pobox.com>
 * @copyright 2014-2019
 * @license MIT-style
 *
 * @param {YUVFrame} buffer - input frame buffer
 * @param {Uint8ClampedArray} output - array to draw RGBA into
 * Assumes that the output array already has alpha channel set to opaque.
 */function c(n,a){var t=n.format.width|0,e=n.format.height|0,R=p(n.format.width/n.format.chromaWidth)|0,u=p(n.format.height/n.format.chromaHeight)|0,f=n.y.bytes,U=n.u.bytes,x=n.v.bytes,l=n.y.stride|0,m=n.u.stride|0,o=n.v.stride|0,_=t<<2,h=0,X=0,I=0,S=0,w=0,g=0,d=0,v=0,C=0,L=0,E=0,P=0,D=0,b=0,F=0,r=0,i=0,T=0;if(R==1&&u==1)for(d=0,v=_,T=0,r=0;r<e;r+=2){for(X=r*l|0,I=X+l|0,S=T*m|0,w=T*o|0,F=0;F<t;F+=2)C=U[S++]|0,L=x[w++]|0,P=(409*L|0)-57088|0,D=(100*C|0)+(208*L|0)-34816|0,b=(516*C|0)-70912|0,E=298*f[X++]|0,a[d]=E+P>>8,a[d+1]=E-D>>8,a[d+2]=E+b>>8,d+=4,E=298*f[X++]|0,a[d]=E+P>>8,a[d+1]=E-D>>8,a[d+2]=E+b>>8,d+=4,E=298*f[I++]|0,a[v]=E+P>>8,a[v+1]=E-D>>8,a[v+2]=E+b>>8,v+=4,E=298*f[I++]|0,a[v]=E+P>>8,a[v+1]=E-D>>8,a[v+2]=E+b>>8,v+=4;d+=_,v+=_,T++}else for(g=0,r=0;r<e;r++)for(i=0,T=r>>u,h=r*l|0,S=T*m|0,w=T*o|0,F=0;F<t;F++)i=F>>R,C=U[S+i]|0,L=x[w+i]|0,P=(409*L|0)-57088|0,D=(100*C|0)+(208*L|0)-34816|0,b=(516*C|0)-70912|0,E=298*f[h++]|0,a[g]=E+P>>8,a[g+1]=E-D>>8,a[g+2]=E+b>>8,g+=4}H.exports={convertYCbCr:c}}(),function(){var p=Y.exports,c=H.exports;function n(a){var t=this,e=a.getContext("2d"),R=null,u=null,f=null;function U(l,m){R=e.createImageData(l,m);for(var o=R.data,_=l*m*4,h=0;h<_;h+=4)o[h+3]=255}function x(l,m){u=document.createElement("canvas"),u.width=l,u.height=m,f=u.getContext("2d")}return t.drawFrame=function(m){var o=m.format;(a.width!==o.displayWidth||a.height!==o.displayHeight)&&(a.width=o.displayWidth,a.height=o.displayHeight),(R===null||R.width!=o.width||R.height!=o.height)&&U(o.width,o.height),c.convertYCbCr(m,R.data);var _=o.cropWidth!=o.displayWidth||o.cropHeight!=o.displayHeight,h;_?(u||x(o.cropWidth,o.cropHeight),h=f):h=e,h.putImageData(R,-o.cropLeft,-o.cropTop,o.cropLeft,o.cropTop,o.cropWidth,o.cropHeight),_&&e.drawImage(u,0,0,o.displayWidth,o.displayHeight)},t.clear=function(){e.clearRect(0,0,a.width,a.height)},t}n.prototype=Object.create(p.prototype),k.exports=n}();var V={exports:{}},j={vertex:`precision lowp float;

attribute vec2 aPosition;
attribute vec2 aLumaPosition;
attribute vec2 aChromaPosition;
varying vec2 vLumaPosition;
varying vec2 vChromaPosition;
void main() {
    gl_Position = vec4(aPosition, 0, 1);
    vLumaPosition = aLumaPosition;
    vChromaPosition = aChromaPosition;
}
`,fragment:`// inspired by https://github.com/mbebenita/Broadway/blob/master/Player/canvas.js

precision lowp float;

uniform sampler2D uTextureY;
uniform sampler2D uTextureCb;
uniform sampler2D uTextureCr;
varying vec2 vLumaPosition;
varying vec2 vChromaPosition;
void main() {
   // Y, Cb, and Cr planes are uploaded as LUMINANCE textures.
   float fY = texture2D(uTextureY, vLumaPosition).x;
   float fCb = texture2D(uTextureCb, vChromaPosition).x;
   float fCr = texture2D(uTextureCr, vChromaPosition).x;

   // Premultipy the Y...
   float fYmul = fY * 1.1643828125;

   // And convert that to RGB!
   gl_FragColor = vec4(
     fYmul + 1.59602734375 * fCr - 0.87078515625,
     fYmul - 0.39176171875 * fCb - 0.81296875 * fCr + 0.52959375,
     fYmul + 2.017234375   * fCb - 1.081390625,
     1
   );
}
`,vertexStripe:`precision lowp float;

attribute vec2 aPosition;
attribute vec2 aTexturePosition;
varying vec2 vTexturePosition;

void main() {
    gl_Position = vec4(aPosition, 0, 1);
    vTexturePosition = aTexturePosition;
}
`,fragmentStripe:`// extra 'stripe' texture fiddling to work around IE 11's poor performance on gl.LUMINANCE and gl.ALPHA textures

precision lowp float;

uniform sampler2D uStripe;
uniform sampler2D uTexture;
varying vec2 vTexturePosition;
void main() {
   // Y, Cb, and Cr planes are mapped into a pseudo-RGBA texture
   // so we can upload them without expanding the bytes on IE 11
   // which doesn't allow LUMINANCE or ALPHA textures
   // The stripe textures mark which channel to keep for each pixel.
   // Each texture extraction will contain the relevant value in one
   // channel only.

   float fLuminance = dot(
      texture2D(uStripe, vTexturePosition),
      texture2D(uTexture, vTexturePosition)
   );

   gl_FragColor = vec4(fLuminance, fLuminance, fLuminance, 1);
}
`};(function(){var p=Y.exports,c=j;function n(a){var t=this,e=n.contextForCanvas(a);if(e===null)throw new Error("WebGL unavailable");function R(r,i){var T=e.createShader(r);if(e.shaderSource(T,i),e.compileShader(T),!e.getShaderParameter(T,e.COMPILE_STATUS)){var s=e.getShaderInfoLog(T);throw e.deleteShader(T),new Error("GL shader compilation for "+r+" failed: "+s)}return T}var u,f,U=new Float32Array([-1,-1,1,-1,-1,1,-1,1,1,-1,1,1]),x={},l={},m={},o,_,h,X,I,S,w,g,d,v;function C(r){return x[r]||(x[r]=e.createTexture()),x[r]}function L(r,i,T,s){var A=C(r);if(e.activeTexture(e.TEXTURE0),n.stripe){var B=!x[r+"_temp"],G=C(r+"_temp");e.bindTexture(e.TEXTURE_2D,G),B?(e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.NEAREST),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,i/4,T,0,e.RGBA,e.UNSIGNED_BYTE,s)):e.texSubImage2D(e.TEXTURE_2D,0,0,0,i/4,T,e.RGBA,e.UNSIGNED_BYTE,s);var y=x[r+"_stripe"],N=!y;N&&(y=C(r+"_stripe")),e.bindTexture(e.TEXTURE_2D,y),N&&(e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.NEAREST),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.NEAREST),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,i,1,0,e.RGBA,e.UNSIGNED_BYTE,D(i)))}else e.bindTexture(e.TEXTURE_2D,A),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.LINEAR),e.texImage2D(e.TEXTURE_2D,0,e.LUMINANCE,i,T,0,e.LUMINANCE,e.UNSIGNED_BYTE,s)}function E(r,i,T){var s=x[r];e.useProgram(f);var A=l[r];A||(e.activeTexture(e.TEXTURE0),e.bindTexture(e.TEXTURE_2D,s),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.LINEAR),e.texImage2D(e.TEXTURE_2D,0,e.RGBA,i,T,0,e.RGBA,e.UNSIGNED_BYTE,null),A=l[r]=e.createFramebuffer()),e.bindFramebuffer(e.FRAMEBUFFER,A),e.framebufferTexture2D(e.FRAMEBUFFER,e.COLOR_ATTACHMENT0,e.TEXTURE_2D,s,0);var B=x[r+"_temp"];e.activeTexture(e.TEXTURE1),e.bindTexture(e.TEXTURE_2D,B),e.uniform1i(S,1);var G=x[r+"_stripe"];e.activeTexture(e.TEXTURE2),e.bindTexture(e.TEXTURE_2D,G),e.uniform1i(I,2),e.bindBuffer(e.ARRAY_BUFFER,o),e.enableVertexAttribArray(_),e.vertexAttribPointer(_,2,e.FLOAT,!1,0,0),e.bindBuffer(e.ARRAY_BUFFER,h),e.enableVertexAttribArray(X),e.vertexAttribPointer(X,2,e.FLOAT,!1,0,0),e.viewport(0,0,i,T),e.drawArrays(e.TRIANGLES,0,U.length/2),e.bindFramebuffer(e.FRAMEBUFFER,null)}function P(r,i,T){e.activeTexture(i),e.bindTexture(e.TEXTURE_2D,x[r]),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.LINEAR),e.uniform1i(e.getUniformLocation(u,r),T)}function D(r){if(m[r])return m[r];for(var i=r,T=new Uint32Array(i),s=0;s<i;s+=4)T[s]=255,T[s+1]=65280,T[s+2]=16711680,T[s+3]=4278190080;return m[r]=new Uint8Array(T.buffer)}function b(r,i){var T=R(e.VERTEX_SHADER,r),s=R(e.FRAGMENT_SHADER,i),A=e.createProgram();if(e.attachShader(A,T),e.attachShader(A,s),e.linkProgram(A),!e.getProgramParameter(A,e.LINK_STATUS)){var B=e.getProgramInfoLog(A);throw e.deleteProgram(A),new Error("GL program linking failed: "+B)}return A}function F(){if(n.stripe){f=b(c.vertexStripe,c.fragmentStripe),e.getAttribLocation(f,"aPosition"),h=e.createBuffer();var r=new Float32Array([0,0,1,0,0,1,0,1,1,0,1,1]);e.bindBuffer(e.ARRAY_BUFFER,h),e.bufferData(e.ARRAY_BUFFER,r,e.STATIC_DRAW),X=e.getAttribLocation(f,"aTexturePosition"),I=e.getUniformLocation(f,"uStripe"),S=e.getUniformLocation(f,"uTexture")}u=b(c.vertex,c.fragment),o=e.createBuffer(),e.bindBuffer(e.ARRAY_BUFFER,o),e.bufferData(e.ARRAY_BUFFER,U,e.STATIC_DRAW),_=e.getAttribLocation(u,"aPosition"),w=e.createBuffer(),g=e.getAttribLocation(u,"aLumaPosition"),d=e.createBuffer(),v=e.getAttribLocation(u,"aChromaPosition")}return t.drawFrame=function(r){var i=r.format,T=!u||a.width!==i.displayWidth||a.height!==i.displayHeight;if(T&&(a.width=i.displayWidth,a.height=i.displayHeight,t.clear()),u||F(),T){var s=function(A,B,G){var y=i.cropLeft/G,N=(i.cropLeft+i.cropWidth)/G,M=(i.cropTop+i.cropHeight)/i.height,W=i.cropTop/i.height,q=new Float32Array([y,M,N,M,y,W,y,W,N,M,N,W]);e.bindBuffer(e.ARRAY_BUFFER,A),e.bufferData(e.ARRAY_BUFFER,q,e.STATIC_DRAW)};s(w,g,r.y.stride),s(d,v,r.u.stride*i.width/i.chromaWidth)}L("uTextureY",r.y.stride,i.height,r.y.bytes),L("uTextureCb",r.u.stride,i.chromaHeight,r.u.bytes),L("uTextureCr",r.v.stride,i.chromaHeight,r.v.bytes),n.stripe&&(E("uTextureY",r.y.stride,i.height),E("uTextureCb",r.u.stride,i.chromaHeight),E("uTextureCr",r.v.stride,i.chromaHeight)),e.useProgram(u),e.viewport(0,0,a.width,a.height),P("uTextureY",e.TEXTURE0,0),P("uTextureCb",e.TEXTURE1,1),P("uTextureCr",e.TEXTURE2,2),e.bindBuffer(e.ARRAY_BUFFER,o),e.enableVertexAttribArray(_),e.vertexAttribPointer(_,2,e.FLOAT,!1,0,0),e.bindBuffer(e.ARRAY_BUFFER,w),e.enableVertexAttribArray(g),e.vertexAttribPointer(g,2,e.FLOAT,!1,0,0),e.bindBuffer(e.ARRAY_BUFFER,d),e.enableVertexAttribArray(v),e.vertexAttribPointer(v,2,e.FLOAT,!1,0,0),e.drawArrays(e.TRIANGLES,0,U.length/2)},t.clear=function(){e.viewport(0,0,a.width,a.height),e.clearColor(0,0,0,0),e.clear(e.COLOR_BUFFER_BIT)},t.clear(),t}n.stripe=function(){return navigator.userAgent.indexOf("Windows")!==-1}(),n.contextForCanvas=function(a){var t={preferLowPowerToHighPerformance:!0,powerPreference:"low-power",failIfMajorPerformanceCaveat:!0,preserveDrawingBuffer:!0};return a.getContext("webgl",t)||a.getContext("experimental-webgl",t)},n.isAvailable=function(){var a=document.createElement("canvas"),t;a.width=1,a.height=1;try{t=n.contextForCanvas(a)}catch{return!1}if(t){var e=t.TEXTURE0,R=4,u=4,f=t.createTexture(),U=new Uint8Array(R*u),x=n.stripe?R/4:R,l=n.stripe?t.RGBA:t.LUMINANCE,m=n.stripe?t.NEAREST:t.LINEAR;t.activeTexture(e),t.bindTexture(t.TEXTURE_2D,f),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_S,t.CLAMP_TO_EDGE),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_T,t.CLAMP_TO_EDGE),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MIN_FILTER,m),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MAG_FILTER,m),t.texImage2D(t.TEXTURE_2D,0,l,x,u,0,l,t.UNSIGNED_BYTE,U);var o=t.getError();return!o}else return!1},n.prototype=Object.create(p.prototype),V.exports=n})(),function(){var p=Y.exports,c=k.exports,n=V.exports,a={FrameSink:p,SoftwareFrameSink:c,WebGLFrameSink:n,attach:function(t,e){e=e||{};var R="webGL"in e?e.webGL:n.isAvailable();return R?new n(t,e):new c(t,e)}};O.exports=a}();var K=O.exports;return K}();
