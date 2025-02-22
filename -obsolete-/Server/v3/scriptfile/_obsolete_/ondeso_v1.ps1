#load Projects in
$projectitems=Get-IniContent "C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\Projects\Projects.txt"
#define get-inicontent function
function Get-IniContent ($filePath)
{
    $ini = @{}
    switch -regex -file $FilePath
    {
        “^\[(.+)\]” # Section
        {
            $section = $matches[1]
            $ini[$section] = @{}
            $CommentCount = 0
        }
        “^(;.*)$” # Comment
        {
            $value = $matches[1]
            $CommentCount = $CommentCount + 1
            $name = “Comment” + $CommentCount
            $ini[$section][$name] = $value
        }
        “(.+?)\s*=(.*)” # Key
        {
            $name,$value = $matches[1..2]
            $ini[$section][$name] = $value
        }
    }
    return $ini

    }



#define dialog
[void] [System.Reflection.Assembly]::LoadWithPartialName("System.Drawing")
[void] [System.Reflection.Assembly]::LoadWithPartialName("System.Windows.Forms")
$objForm = New-Object System.Windows.Forms.Form
$objForm.Backcolor="white"
$objForm.TopMost=$true
$objForm.ControlBox=$false
$objForm.MaximizeBox=$false
$objForm.MinimizeBox=$false


##Backgroundlogo##
$objForm.BackgroundImageLayout = 0
$objForm.BackgroundImage =[System.Drawing.Image]::FromFile('C:\ProgramData\ondeso\SecureRelease\Workplace\banner\customlogo.png')


$objForm.StartPosition = "CenterScreen"
$objForm.Size = New-Object System.Drawing.Size(350,500)
#set ico
$objForm.Icon="C:\Program Files (x86)\ondeso\SecuRelSuite\Ondeso.ico"
$objForm.Text = ""

$objComboBoxselProject = New-Object System.Windows.Forms.ComboBox
$objComboBoxselProject.Location = New-Object System.Drawing.Size(30,50)
$objComboBoxselProject.Size = New-Object System.Drawing.Size(200,20)
#$objComboBoxselProject.Font = New-Object System.Drawing.Font("Arial", 10)
$objComboBoxselProject.Text= "Select Project"

    foreach ($item in $projectItems.Project.Keys) {
        $objComboBoxselProject.Items.Add($item)
    }

#eventhandle Project dropdown changed
$objComboBoxselProject.add_SelectedIndexChanged({

    #clear box
    $OKButton.Enabled = $false
    $objListBox.Items.Clear()
    $OKButton.BackColor = "lightgray"
    $objComboBoxselMaintain.Items.Clear()
    $objComboBoxselMaintain.Text = "Select maintenance group"

    #load maintain
    $Global:selproject=$objComboBoxselProject.SelectedItem
    $Global:projectItemValue = $projectitems.Project.$selproject
    $Global:Maintainitem= Get-IniContent "C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\Projects\$projectItemValue.txt"

        foreach ($item in $Maintainitem.MaintainGroup.Keys) {
            $objComboBoxselMaintain.Items.Add($item)
        }

#Write-Host $selproject
#Write-Host $projectItemValue
#Write-Host $Maintainitem


})

$objForm.Controls.Add($objComboBoxselProject)

$objComboBoxselMaintain = New-Object System.Windows.Forms.ComboBox
$objComboBoxselMaintain.Location = New-Object System.Drawing.Size(30,80)
$objComboBoxselMaintain.Size = New-Object System.Drawing.Size(200,20)
#$objComboBoxselMaintain.Font = New-Object System.Drawing.Font("Arial", 10)
$objComboBoxselMaintain.Text= "Select maintenance group"


$objForm.Controls.Add($objComboBoxselMaintain)

    #eventhandle Maintain dropdown changed

    $objComboBoxselMaintain.add_SelectedIndexChanged({
    $OKButton.Enabled = $false
    $objListBox.Items.Clear()
    $OKButton.BackColor = "lightgray"
    $selMaintain=$objComboBoxselMaintain.SelectedItem
    $Global:MaintainItemValue= $Global:Maintainitem.MaintainGroup.$selMaintain

    $Global:MaintainItemValuesplit=$MaintainItemValue.Split(";")    
    $MaintainItemValuesplit


#Write-Host $Global:projectItemValue
#Write-Host $Global:MaintainItemValue



})

#Combobox select Computername
$objLabelSelectComputerName = New-Object System.Windows.Forms.Label
$objLabelSelectComputerName.Location = New-Object System.Drawing.Size(30,160)
$objLabelSelectComputerName.Size = New-Object System.Drawing.Size(130,15)
$objLabelSelectComputerName.Text = "Enter Computername:"
$objForm.Controls.Add($objLabelSelectComputerName)

$objTextBoxEnterComputerName = New-Object System.Windows.Forms.Textbox
$objTextBoxEnterComputerName.Location = New-Object System.Drawing.Size(30,175)
$objTextBoxEnterComputerName.Size = New-Object System.Drawing.Size(200,20)
$objTextBoxEnterComputerName.Text= ""

$objForm.Controls.Add($objTextBoxEnterComputerName)


#search Button 
    $SearchButton = New-Object System.Windows.Forms.Button
    $SearchButton.Location = New-Object System.Drawing.Size(235,174)
    $SearchButton.Size = New-Object System.Drawing.Size(75,23)
    $SearchButton.Text = "Search"
    $SearchButton.Name = "Search"
    $SearchButton.Add_Click({
    $OKButton.Enabled = $false
    $objListBox.Items.Clear()
    $OKButton.BackColor = "lightgray"

   try{

    $computernametoSearch=$objTextBoxEnterComputerName.Text
    $ou=$Global:MaintainItemValuesplit[0]
    
        #$ldapPath = "LDAP://OU=Clients,<DN>"
        $ldapPath = "LDAP://OU=Clients,OU=$ou,OU=E067,DC=ondeso-sr,DC=com"
        $searchRoot = New-Object System.DirectoryServices.DirectoryEntry($ldapPath)
        $searcher = New-Object System.DirectoryServices.DirectorySearcher($searchRoot)
        $searcher.Filter = "(name=*$computernametoSearch*)"
        $result = $searcher.FindAll()
        $adComputername = $result.properties.name

       $objListBox.Items.AddRange($adComputername)

       }

   catch{
   

   }
    
    })
    $objForm.Controls.Add($SearchButton)

#list box 

    $objLabelSearchResult = New-Object System.Windows.Forms.Label
    $objLabelSearchResult.Location = New-Object System.Drawing.Size(30,205)
    $objLabelSearchResult.Size = New-Object System.Drawing.Size(130,20)
    $objLabelSearchResult.Text = "Search Result:"
    $objForm.Controls.Add($objLabelSearchResult)

    $objListBox = New-Object System.Windows.Forms.ListBox
    $objListBox.Location = New-Object System.Drawing.Size(30,225)
    $objListBox.Size = New-Object System.Drawing.Size(150,200)
    $objListBox.AutoSize = $false
    $objListBox.SelectionMode = "one"
    $objListBox.Sorted = $true
    $objForm.Controls.Add($objListBox)

    $objListBox.Add_SelectedIndexChanged({
    if ($objListBox.SelectedIndex -ne -1) {
        $OKButton.Enabled = $true 
        $OKButton.BackColor = "white"
    } else {
        $OKButton.Enabled = $false 
    }
})

   

#OK Button 
    $OKButton = New-Object System.Windows.Forms.Button
    $OKButton.Location = New-Object System.Drawing.Size(30,430)
    $OKButton.Size = New-Object System.Drawing.Size(75,23)
    $OKButton.Text = "OK"
    $OKButton.Name = "OK"
    $OKButton.Enabled = $false
    $OKButton.BackColor = "lightgray"
    $OKButton.Add_Click({
    
    $Type=$Global:MaintainItemValuesplit[1]
    $Hostname=$objListBox.Items
    $Project=$Global:selproject

    Out-file -FilePath C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\StartIntegration.ini -InputObject "[Basic]" -Append
    Out-file -FilePath C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\StartIntegration.ini -InputObject "Hostname=$Hostname" -append
    Out-file -FilePath C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\StartIntegration.ini -InputObject "Project=$Project" -append
    Out-file -FilePath C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\StartIntegration.ini -InputObject "Type=$Type" -append
    Out-file -FilePath C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\temp\Ready.txt 
    Write-Host $objListBox.Items
    $objForm.Close()
    
    })

    $objForm.Controls.Add($OKButton)




#CancelButton
    $CancelButton = New-Object System.Windows.Forms.Button
    $CancelButton.Location = New-Object System.Drawing.Size(250,430)
    $CancelButton.Size = New-Object System.Drawing.Size(75,23)
    $CancelButton.Text = "Cancel"
    $CancelButton.Name = "Cancel"
    $CancelButton.Add_Click({
    
        $Result = [System.Windows.Forms.MessageBox]::Show("Abort?","Abort",4,[System.Windows.Forms.MessageBoxIcon]::Exclamation)
 
            If ($Result -eq "Yes")

                {
                Out-File C:\ProgramData\ondeso\SecureRelease\Workplace\Wizard\temp\CancelProcessing.txt
                $objForm.Close()
                }
            else
                {
                   
                }

    })
    #$CancelButton.Add_Click({$objForm.Close()})
    $objForm.Controls.Add($CancelButton) 


    

#show dialog
[void] $objForm.ShowDialog()