# Adhoc Job Related Scripts
Hosts a set of scripts used for mitigating, maintaining and managing jobs manually in the times of peril.

Scripts:
1. <b>Ray Terminator :</b><br>
This script will terminate all running ray clusters in a given environment (eg: <i>dev-5</i>, <i>dev-4</i>)<br>
This script has explicit check to abort if the environment is <i><b>production</b></i>.<br><br>
<b>Usage</b>:<br>
   1. Install mactl tool (if not present)<br>
       ```
       brew update && brew install mactl
       ```
   2. Run cerberus command on a different tab to tunnel to MA apiserver
        ```
        cerberus -r michelangelo-apiserver-<env>
        ```
      Eg :
        ```
        cerberus -r michelangelo-apiserver-dev-5
        ```
   3. Update `env` global variable in the script as per need<br><br>
   4. Run the scipt :
        ```
        python3 ray_terminator.py
        ```
      