import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const editor=readFileSync(new URL('../web/featuremods/editor.js',import.meta.url),'utf8');
const compare=readFileSync(new URL('../web/featuremods/filecompare.js',import.meta.url),'utf8');
const completion=readFileSync(new URL('../web/featuremods/editorcompletion.js',import.meta.url),'utf8');

function between(source,begin,end){
  const a=source.indexOf(begin),b=source.indexOf(end,a+begin.length);
  assert.ok(a>=0&&b>a,'missing source section '+begin);
  return source.slice(a,b);
}
const registry=between(editor,'const editorLanguageRegistry=[','function editorIsMarkdown(');
const lookup=between(editor,'function editorLanguageDefinition(pathValue){','const legacyKeywordSets=');
const {languageForPath}=vm.runInNewContext(
  'function basename(p){return String(p).split("/").pop();}\n'+registry+lookup+
  '\n({languageForPath:editorLanguageDefinition})'
);

test('standard TypeScript, TSX and Swift remain unchanged',()=>{
  assert.equal(languageForPath('src/main.ts').id,'typescript');
  assert.equal(languageForPath('src/components/view.tsx').id,'tsx');
  assert.equal(languageForPath('src/types/fgui.d.ts').id,'typescript');
  assert.equal(languageForPath('src/Player.swift').id,'swift');
});

test('Cocos Creator serialized text assets use JSON mode',()=>{
  for(const extension of ['prefab','scene','fire','anim','meta']){
    const mode=languageForPath('/game/assets/Characters/Player.'+extension);
    assert.equal(mode.id,'cocosjson',extension);
    assert.equal(mode.codeMirror,'json',extension);
  }
});

test('FairyGUI project file is XML and binary packages are not misread as text',()=>{
  const mode=languageForPath('UIProject/SomeUI.fairy');
  assert.equal(mode.id,'fairygui');
  assert.equal(mode.codeMirror,'xml');
  assert.equal(languageForPath('UIProject/assets/Bag/package.xml').id,'xml');
  assert.equal(languageForPath('assets/resources/UI/Bag.bin'),null);
  assert.equal(languageForPath('assets/resources/UI/Bag.fui'),null);
});

test('File Compare aliases specialized formats to built-in XML/JSON token modes',()=>{
  const declaration=between(compare,'function compareLanguageID(pathValue){','function compareSyntaxProfile(');
  const getId=vm.runInNewContext(declaration+'\ncompareLanguageID',{
    TaskMenuEditor:{languageForPath}
  });
  assert.equal(getId('game/assets/Main.prefab'),'json');
  assert.equal(getId('game/UIProject/Example.fairy'),'xml');
  assert.equal(getId('game/scripts/Main.ts'),'typescript');
});

test('Cocos TypeScript detection is opt-in based on framework usage',()=>{
  const declaration=between(completion,'function isCocosTypeScriptDocument(doc){','function languageForFile(file)');
  const detect=vm.runInNewContext(declaration+'\nisCocosTypeScriptDocument');
  const doc=text=>({length:text.length,sliceString:(from,to)=>text.slice(from,to)});
  assert.equal(detect(doc("import { _decorator } from 'cc';")),true);
  assert.equal(detect(doc("import * as fgui from 'fairygui-cc';")),true);
  assert.equal(detect(doc('const { ccclass } = cc._decorator;')),true);
  assert.equal(detect(doc('fgui.UIPackage.createObject("UI","Main")')),true);
  assert.equal(detect(doc('import { Component } from "./component";')),false);
  assert.equal(detect(doc('const description = "an unrelated TypeScript program"')),false);
});

test('Cocos completion remains source gated and does not replace TypeScript completion',()=>{
  assert.match(completion,/cocosOptions=p===profiles\.typescript\?/);
  assert.match(completion,/isCocosTypeScriptDocument\(context\.state\.doc\)/);
  assert.match(completion,/options:cocosOptions\.length&&/);
  assert.match(completion,/snippet\('ccComponent3'/);
  assert.match(completion,/snippet\('ccComponent2'/);
  assert.match(completion,/snippet\('fguiCreateObject'/);
});
