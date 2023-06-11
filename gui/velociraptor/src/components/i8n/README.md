# Updating and maintaining the language files

There is a script that can be used to maintain translation files:

1. run the script on each language file:

```
$ python3 scripts/find_i8n_translations.py ./gui/velociraptor/src/components/i8n/de.jsx
Wrote json file ./gui/velociraptor/src/components/i8n/de_new.json with 6 entries
Wrote automated json file ./gui/velociraptor/src/components/i8n/de_automated.json with 163 entries
```

This will generate a number of files for each language:

* The `.jsx` file will be included in the build. That file contains
  *curated* translations that a fluent speaker in the language has
  verified. e.g. `de.jsx`

* The `de.json` file contains automated translations- the keys are
  obfuscated as hex encoding in order to protect them from Google
  translate. The GUI will use these translations in addition to the
  ones in `de.jsx`.

* The `de_new.json` file contains additional translations the script
  above has detected are missing. This JSON file contains a keys that
  are obfuscated hex encoded and values are English language phrases
  to be translated.

  When new words are found, you should copy the JSON object from
  `de_new.json` into Google translate and select the relevant target
  language. Then copy the translated strings back into `de.json`. For
  example, `de_new.json` might contain

```
    "436c69636b20746f2076696577206f722065646974": "Click to view or edit",
```
  Paste this in Google translate and copy the translation back to `de.json`:

```
    "436c69636b20746f2076696577206f722065646974": "Zum Anzeigen oder Bearbeiten klicken",
```

  Running the script again will remove the translation from
  `de_new.json`. We aim to have an empty `de_new.json` as all
  translations will be added to `de.json`

* Ideally we consider automated translations to be inferior to
  manually audited translations made by fluent speakers of the target
  language. Therefore we aim to eventually move automated translations
  to `de.jsx` after review. The above script creates
  `de_automated.json` containing English keys and target language
  values for review by fluent speakers.

  If you are a fluent speaker, simply review the translations in
  `de_automated.json` and copy the corrected translations directly
  into `de.jsx`. You can then remove these from `de.json`


So to summarize:

1. The highest confidence translations are in `de.jsx` and were
   reviewed by a fluent speaker of the target language.
2. Automated translations are added to that in `de.json` after being
   translated by Google translate.
3. The above script audits the source code each time it is run for
   missing translations, adding then to `de_new.json`.
4. If you are a maintainer, feed `de_new.json` into Google translate
   and save the results in `de.json`
5. If you are a native speaker, review `de_automated.json` and paste
   the translations in `de.jsx` directly, while removing them from
   `de.json`


NOTE: This entire process can be achieved using `make translations`
