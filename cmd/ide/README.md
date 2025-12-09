# Silky IDE

The Silky IDE provides an interactive environment to develop and test Silky YAML configurations.

![ide](../../assets/ide_showcase.gif)

---

## How it works

* The IDE prompts the user to select a YAML configuration file.
* It loads the selected configuration and watches the file for changes.
* Every time the configuration is modified and saved, the IDE automatically restarts the crawling process.
* The IDE displays a log window showing:

  * The list of intermediate steps executed
  * The final output of the crawler
* By selecting any intermediate step, users can inspect:

  * The context associated with that step
  * The partial result produced at that point in the crawling process

---

## Additional Features

* Users can stop the crawling process at any time.
* When stopped, the IDE can dump the entire step tree and results into the `/out` folder for offline inspection and debugging.

---

## Summary

The Silky IDE streamlines configuration development by providing real-time execution feedback, detailed inspection of intermediate states, and convenient output dumping — all aimed at making your Silky crawling configurations easier to build, debug, and optimize.

