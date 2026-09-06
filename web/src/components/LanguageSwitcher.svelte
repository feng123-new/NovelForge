<script lang="ts">
  import { locale, applyLocale, isLocale } from '../lib/i18n';
  export let id = 'workspace-language';
  function change(event: Event) {
    const value = (event.currentTarget as HTMLSelectElement).value;
    if (isLocale(value)) applyLocale(value);
  }
</script>
<label class="form-control" for={id}>
  <span class="sr-only">{$locale === 'zh-CN' ? '界面语言' : 'Interface language'}</span>
  <select {id} class="select select-bordered select-sm max-w-36" value={$locale} on:change={change} data-testid={id}>
    <option value="zh-CN">简体中文</option>
    <option value="en">English</option>
  </select>
</label>
