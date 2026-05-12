<script setup lang="ts">
import AppHeader from './AppHeader.vue'
import AppSidebar from './AppSidebar.vue'
</script>

<template>
  <div class="app-layout">
    <AppHeader />
    <div class="main-area">
      <AppSidebar />
      <main class="content">
        <slot />
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-layout { display: flex; flex-direction: column; height: 100vh; }
.main-area { display: flex; flex: 1; overflow: hidden; }
.content { flex: 1; overflow-y: auto; background: #f0f2f5; }
</style>
