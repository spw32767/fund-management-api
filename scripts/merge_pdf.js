#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

async function loadPdfLib() {
  try {
    return require('pdf-lib');
  } catch (error) {
    console.error('Unable to load pdf-lib. Ensure NODE_PATH includes a directory containing pdf-lib.');
    throw error;
  }
}

async function mergePDFs(outputPath, inputPaths) {
  if (inputPaths.length === 0) {
    throw new Error('No input PDF files provided');
  }

  const { PDFDocument } = await loadPdfLib();
  const mergedPdf = await PDFDocument.create();

  for (const input of inputPaths) {
    const absolutePath = path.resolve(input);
    const data = await fs.promises.readFile(absolutePath);
    const src = await PDFDocument.load(data);
    const pages = await mergedPdf.copyPages(src, src.getPageIndices());
    for (const page of pages) {
      mergedPdf.addPage(page);
    }
  }

  const mergedBytes = await mergedPdf.save();
  await fs.promises.writeFile(path.resolve(outputPath), mergedBytes);
}

async function main() {
  const args = process.argv.slice(2);
  if (args.length < 2) {
    console.error('Usage: merge_pdf.js <output> <input1> <input2> ...');
    process.exit(1);
  }

  const [output, ...inputs] = args;
  try {
    await mergePDFs(output, inputs);
  } catch (error) {
    console.error(error && error.stack ? error.stack : error.message || String(error));
    process.exit(1);
  }
}

main();