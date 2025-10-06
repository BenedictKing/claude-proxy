#!/usr/bin/env node
/**
 * 构建后处理脚本
 * 用于验证构建产物并提供友好的错误提示
 */

const fs = require('fs');
const path = require('path');

const rootDir = path.join(__dirname, '..');
const frontendDistPath = path.join(rootDir, 'frontend', 'dist');
const backendDistPath = path.join(rootDir, 'backend', 'dist');

console.log('\n📦 构建后验证...\n');

// 检查前端构建产物
const frontendIndexPath = path.join(frontendDistPath, 'index.html');
if (fs.existsSync(frontendIndexPath)) {
  console.log('✅ 前端构建成功: frontend/dist/');

  // 统计文件数量
  const files = fs.readdirSync(frontendDistPath, { recursive: true });
  console.log(`   文件数量: ${files.length}`);
} else {
  console.warn('⚠️  前端构建产物未找到: frontend/dist/index.html');
  console.warn('   前端Web界面可能无法访问');
}

// 检查后端构建产物
const backendServerPath = path.join(backendDistPath, 'server.js');
if (fs.existsSync(backendServerPath)) {
  console.log('✅ 后端构建成功: backend/dist/');
} else {
  console.warn('⚠️  后端构建产物未找到: backend/dist/server.js');
}

console.log('\n💡 部署提示:');
console.log('   • Docker部署: 前端资源会自动复制到 /app/frontend/dist');
console.log('   • 本地运行: 直接从项目根目录运行 "bun run start"');
console.log('   • 如遇到前端404: 检查 frontend/dist 目录是否存在');
console.log('\n🚀 构建验证完成!\n');
