import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const editor=readFileSync(new URL('../web/featuremods/editor.js',import.meta.url),'utf8');
const diff=readFileSync(new URL('../web/featuremods/filecompare.js',import.meta.url),'utf8');
const section=(a,b)=>{
  const start=editor.indexOf(a),end=editor.indexOf(b,start+a.length);
  assert.ok(start>=0&&end>start,'source marker missing '+a);
  return editor.slice(start,end);
};
const registry=section('const editorLanguageRegistry=[','function editorIsMarkdown(');
const lookup=section('function editorLanguageDefinition(pathValue){','const legacyKeywordSets=');
const languageForPath=vm.runInNewContext(
 'function basename(p){return String(p||"").split("/").pop()||p;}\n'+registry+lookup+'\neditorLanguageDefinition'
);
test('Gradle Groovy, Kotlin DSL and ordinary Kotlin are distinguishable',()=>{
  assert.equal(languageForPath('build.gradle').id,'gradle');
  assert.equal(languageForPath('settings.gradle').legacy,'groovy');
  assert.equal(languageForPath('scripts/custom.groovy').id,'gradle');
  assert.equal(languageForPath('build.gradle.kts').id,'gradle-kotlin');
  assert.equal(languageForPath('settings.gradle.kts').label,'Gradle Kotlin DSL');
  assert.equal(languageForPath('src/Main.kt').id,'kotlin');
});
test('structured data and build configs receive recognized language modes',()=>{
  for(const [path,id] of [
    ['gradle.properties','config'],['.editorconfig','config'],['settings.xml','xml'],
    ['config.json','json'],['data.csv','csv'],['data.tsv','tsv'],
    ['proguard-rules.pro','proguard'],['some.cfg','config'],
    ['versions.toml','toml'],['pipeline.yaml','yaml']
  ])assert.equal(languageForPath(path).id,id,path);
});
test('ProGuard and Gradle diff syntax includes comment rules and keyword profiles',()=>{
  assert.ok(diff.includes("gradle:'def class package import"));
  assert.ok(diff.includes("if(id==='proguard')profile.line=['#']"));
  assert.ok(diff.includes("const cLike=new Set(['cpp'"));
  assert.ok(diff.includes("'gradle'"));
});
