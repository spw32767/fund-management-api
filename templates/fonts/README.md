# Publication reward preview fonts

This directory packages the font families LibreOffice needs when rendering the publication reward template server side.

* `THSarabunNew/` mirrors the four TH Sarabun New faces already shipped with the frontend so the preview pipeline can resolve the template's primary Thai family without depending on the web build output.
* Additional font families (for example Cordia New or Angsana) can be copied into subdirectories here; `collectFontDirectories()` will automatically expose them to LibreOffice during conversion.