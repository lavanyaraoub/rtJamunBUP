
var HOST="http://192.168.202.1:5000";
//var HOST="http://192.168.0.101:5000";
var FILE_CONTENT_API = HOST+"/getContents";
var GET_PROGRAMS_API = HOST+"/programs";
var CREATE_PROGRAM_API  = HOST+"/createFile";
var DAC_SETTINGS_API = HOST+"/dac_params";
var RENAME_API = HOST+"/renameFile";
var DELETE_API = HOST+"/deleteFile";
var sl = false;
var ecs;
var current_line = 1;
var prev_line = -1;


function validateProgram(code){
  code = code.toUpperCase();
  var lines = code.split("\n");
  var pattern = new RegExp(/^[-AGRDFMP0123456789;.\s]+$/i)
  for(var i=0; i<lines.length; i++){
    var line = lines[i];
    //console.log(line);
    if(pattern.test(line) !== true){
      return {'status' : 'error', 'line':i+1};
    }
  }
  return {'status' : 'success'};
}

angular.module('starter.controllers', [])
.factory('socket', ['$rootScope', function($rootScope) {
  var socket = io.connect('http://192.168.202.1:9090/');

  return {
    on: function(eventName, callback){
      socket.on(eventName, callback);
    },
    emit: function(eventName, data) {
      socket.emit(eventName, data);
    }
  };
}])

.controller('AppCtrl', function($scope, $ionicModal, $timeout,$location,$ionicModal) {
  $scope.loginData = {};
  $scope.path = "";
  //console.log("current location",$location.path());
})

.controller('PositionCtrl', function($scope, $http, socket, $ionicModal, $location){
  //console.log("Inside Position Controller");
  $scope.p = {x_dest:'', x_cur:''}
  $scope.p.x_dest = 0;
  $scope.p.x_cur = 0;
  $scope.mx_cur = 0;
  $scope.pot = "images/off.png";
  $scope.not = "images/off.png";
  $scope.home = "images/off.png";
  $scope.ecs = "images/off.png";
  $scope.cl = "images/off.png";
  $scope.dcl = "images/off.png";
  $scope.fin = "images/off.png";
  $scope.almin = "images/off.png";
  $scope.fault_img = "images/green_l.png";
  $scope.name = "Home";
  $scope.$watch(function(){
      $scope.path = $location.path();
      ////console.log("Path inside watch --> "+$scope.path);
      if($scope.path.indexOf('home') > 0){
        $scope.name = "Home";
      }else if($scope.path.indexOf('manual') > 0){
        $scope.name = "Manual";
      }
      else if ($scope.path.indexOf('program') > 0) {
        $scope.name = "Program Manager";
      }
      else if ($scope.path.indexOf('auto') > 0) {
        $scope.name = "Auto";
      }
      else if ($scope.path.indexOf('setting') > 0) {
        $scope.name = "Settings";
      }
      else if ($scope.path.indexOf('drive_params') > 0) {
        $scope.name = "Machine Params";
      }else if ($scope.path.indexOf('pitch_error') > 0) {
        $scope.name = "Pitch Error";
      }

  }, function(value){
      //console.log(value);
  });


  $ionicModal.fromTemplateUrl('my-modal.html', {
    scope: $scope,
    animation: 'slide-in-up'
  }).then(function(modal) {
    $scope.pos_modal = modal;
  });
  $scope.pos_openModal = function() {
    $scope.pos_modal.show();
  };
  $scope.pos_closeModal = function() {
    $scope.pos_modal.hide();
  };
  // Cleanup the modal when we're done with it!
  $scope.$on('$destroy', function() {
    $scope.pos_modal.remove();
  });




  //var p ={pot:"images/off.png", not:"images/off.png"};
  /*for(var i=0; i<=5; i++){
    p[i] = "images/off.png";
  }*/
  $scope.emergency = false;
  $scope.toggleEmergency = function(){
    $scope.emergency = !$scope.emergency;
    //alert("Emergency status -> "+$scope.emergency);
    socket.emit("emergency",{"status":$scope.emergency});
  }

  socket.on('pos_data', function(data){
    //console.log("Position Data",data);
    var pos = data.data;
    pos = Math.round(pos*10000)/10000;
    var obj = document.getElementById('x_dest');
    var obj1 = document.getElementById('x1_dest');
    if(obj){
      obj.innerHTML = pos;
    }
    if(obj1){
      obj1.innerHTML = pos;
    }
  });

  socket.on("destination_position", function(data){
    //console.log(data);
    var pos = data.pos;
    if(pos === null || pos === undefined){
      return;
    }
    if(Object.keys(data).length == 0){
      return;
    }
    pos = Math.round(pos*10000)/10000;
    var rnd = Math.round(pos);
    if(Math.abs(rnd-pos) <= 0.001){
       pos = rnd;
    }
    pos = pos % 360;
    //console.log(Math.round(a));


    ////console.log(pos);
    //code_content
    //$(".code_content").scrollTop()
    ////console.log(pos);
    var t_obj = document.getElementById('x_cur');
    var t1_obj = document.getElementById('x1_cur');
    ////console.log(t_obj);
    t_obj.innerHTML = pos.toString();
    if(t1_obj){
      t1_obj.innerHTML = pos.toString();
    }
    //t_obj.textContent = " "+pos+" ";
    /*if(obj){
      obj.innerHTML = pos;
    }
    $scope.x_cur = pos;*/
  })

  socket.on("line_number", function(data){
    //console.log("Line Number details --> ");
    //console.log(data);
    var ln = parseInt(data.line);
    $('.code_content').animate({scrollTop: 40*ln}, 20);
    var obj = document.getElementById("num"+ln);
    if(prev_line >=0){
      //console.log("Obtained prev_line "+prev_line);
      var tmp_obj = document.getElementById("num"+prev_line);
      tmp_obj.style.background = "#1f233a";
    }
    if(obj){
      obj.style.background = "rgba(45, 204, 112, 0.52)";
    }
    var obj2 = document.getElementById("lineno");
    obj2.innerHTML = ln+1;
    prev_line = ln;
  })
  //rgba(45, 204, 112, 0.52)
  $scope.resetAlarms = function(){
    socket.emit('reset',{'status':'clear'})
  }

  socket.on("alarms", function(message){
    ////console.log("Alarm Data", message);
    var r = message.split(" ");
    $scope.$apply(function(){
      $scope.home = (r[1][2] == 1) ? "images/on.png" : "images/off.png";
      $scope.ecs = (r[1][3] == 1) ? "images/on.png" : "images/off.png";
      $scope.cl = (r[1][4] == 1) ? "images/on.png" : "images/off.png";
      $scope.dcl = (r[1][5] == 1) ? "images/on.png" : "images/off.png";
      $scope.not = (r[1][0] == 1) ? "images/on.png" : "images/off.png";
      $scope.pot = (r[1][1] == 1) ? "images/on.png" : "images/off.png";
    })
    /*for(var j=0;j<r[1].length;j++){
      var d = document.getElementById('alarm_x_'+j);
      if(d){
        d.style.backgroundColor = (r[1][j] == 1) ? "red" : "green";
      }
    }*/
  });
  socket.on("alarm_error", function(message){
    if((message.startsWith("No Alarms"))){
      $scope.almin = "./images/off.png";
      $scope.fault_img = "./images/green_l.png";
    }
    else{
      $scope.almin = "./images/on.png";
      $scope.fault_img = "./images/red_l.png";
    }

    $scope.alarm_message = message;

  });
  socket.on("FINSIGNAL",function(){
    $scope.fin = "images/on.png";
    //console.log("FINSIGNAL RECEIVED");
  });



  $ionicModal.fromTemplateUrl('templates/io-sheet.html', {
      scope: $scope,
      viewType: 'bottom-sheet',
      animation: 'slide-in-up'
    }).then(function(modal) {
      $scope.modal = modal;
  });

  $ionicModal.fromTemplateUrl('templates/alarm-sheet.html', {
      scope: $scope,
      viewType: 'bottom-sheet',
      animation: 'slide-in-up'
    }).then(function(modal) {
      $scope.a_modal = modal;
  });


})
.controller('HomeCtrl', function($scope, $http, $state,socket, $location, $ionicModal){
  $scope.p={};
  $scope.max_rpm = 20;
  $http.get(DAC_SETTINGS_API)
     .success(function(data){
       //console.log(data);
       //alert("Settings fetch Complete");
       $scope.max_rpm = data.resp.X.max_rpm;
     })
   .error(function(err){
     //console.log(err);
   })
  //socket.emit("step_mode",{status:0});
  $scope.refreshbutton = function(){
    location.reload();
  }
  $ionicModal.fromTemplateUrl('mdi_program.html', {
    scope: $scope,
    animation: 'slide-in-up'
  }).then(function(modal) {
    $scope.modal = modal;
    //$scope.modal.show();
  });
  $scope.number = 5;
  $scope.getNumber = function(num) {
    return new Array(num);
  }
  $scope.openModal = function() {
    $scope.modal.show();
    $scope.p.message = "";
  };
  $scope.closeModal = function() {
    $scope.modal.hide();
  };

  // Cleanup the modal when we're done with it!
  $scope.$on('$destroy', function() {
    $scope.modal.remove();
  });

  $scope.zeroRef = function(){
    //console.log("Triggered Zero Ref");
    socket.emit('goToZero', {});
  }

  $scope.stop_execution = function(){
    //console.log("Inside Stop Execution. Will stop executions now..");
    socket.emit("stop_execution", {"status":"stop"});
  }
  $scope.run_mdi = function(){
    //console.log($scope.p.pgm_code_area);
    var resp = validateProgram($scope.p.pgm_code_area);
    //console.log(resp);
    if(resp.status == "error"){
      //console.log("Error received");
      $scope.message = "Invalid syntax at line number "+resp.line;
      return;
    }

    var reg = /f(\d+)/gi;
    var fr = $scope.p.pgm_code_area.match(reg);
    for(var i=0; i<fr.length; i++){
      var rpm = fr[i].slice(1, fr[i].length);
      console.log($scope.max_rpm);
      if(rpm > $scope.max_rpm){
        $scope.message = "Max supported RPM is "+$scope.max_rpm+", at "+fr[i];
        return;
      }
    }
    $scope.message = "";
    $http.post(CREATE_PROGRAM_API, {file_name:"TEMP_DEL_FILE", contents:$scope.p.pgm_code_area})
      .success(function(data){
        //console.log("File created succesfully");
        $scope.message  = "Executing program now.";
        socket.emit("execute", {file_name:"TEMP_DEL_FILE", mode:"1", ecs:0});
        //$scope.getPrograms();
      })
      .error(function(data){
        //console.log("Program Creation Failed. Please re-try.");
        //console.log("Failed to save Program");
      });

  }

  //console.log("Inside Home");
  $scope.path = "";
  $scope.show = false;
  $scope.$watch(function(){
      $scope.path = $location.path();
      ////console.log("pathchanged",$scope.path);
      if($scope.path.indexOf('home') > 0 || $scope.path.indexOf('settings') > 0){
        $scope.show = false;
      }else{
        $scope.show = true;
      }
  }, function(value){
      //console.log(value);
  });

  $scope.keyboard_options = {numberPad:true,showInMobile:true,
                            forcePosition:'bottom',customClass:'kb_style'};
  $scope.next = function(destination){
    //console.log(destination);
    if(destination == "manual"){
      $state.go("app.manual");
    }else if(destination == "program"){
      $state.go("app.program");
    }else if(destination == "auto"){
      $state.go("app.auto");
    }else if(destination == "settings"){
      $state.go("app.settings");
    }else{
      $state.go("app.home");
    }
  }
  $scope.p={feed_rate : 10, step_count:0.1};
  $scope.display_text = "START JOG";
  $scope.step_btn_01 = "step_btn_selected";
  $scope.step_btn_001 = "step_btn";
  $scope.step_btn_0001 = "step_btn";
  $scope.dir = 1;
  $scope.dir1_img = "images/left-pad-pressed.png";
  $scope.dir2_img = "images/right-pad.png";
  $scope.mode = "jog";
  $scope.step_mode_button = "mode_button";
  $scope.jog_mode_button = "mode_button_active";
  $scope.setMode = function(mode){
    if(mode == "step"){
      $scope.mode="step";
      $scope.step_mode_button = "mode_button_active";
      $scope.jog_mode_button = "mode_button";
      //Update the drive mode to step..
      socket.emit('enable_step_mode',{status:'1'});
    }
    else{
      $scope.mode="jog";
      $scope.step_mode_button = "mode_button";
      $scope.jog_mode_button = "mode_button_active";
    }
  }
  $scope.setStep = function(val){
    if(val == 1){
      $scope.p.step_count = 0.1;
      $scope.step_btn_01 = "step_btn_selected";
      $scope.step_btn_001 = "step_btn";
      $scope.step_btn_0001 = "step_btn";

    }else if (val == 2) {
      $scope.p.step_count = 0.01;
      $scope.step_btn_01 = "step_btn";
      $scope.step_btn_001 = "step_btn_selected";
      $scope.step_btn_0001 = "step_btn";
    }else{
      $scope.p.step_count = 0.001;
      $scope.step_btn_01 = "step_btn";
      $scope.step_btn_001 = "step_btn";
      $scope.step_btn_0001 = "step_btn_selected";
    }
  }
  $scope.setDirection = function(dir){
    if(dir == 1){
      $scope.pos_dir = "active";
      $scope.neg_dir = "passive";
      $scope.dir = 1;
      $scope.dir1_img = "images/left-pad-pressed.png";
      $scope.dir2_img = "images/right-pad.png";
    }else{
      $scope.neg_dir = "active";
      $scope.pos_dir = "passive";
      $scope.dir = 0;
      $scope.dir1_img = "images/left-pad.png";
      $scope.dir2_img = "images/right-pad-pressed.png";
    }
  }


  $scope.start_operation = function(){
    //console.log($scope.mode);
    //console.log($scope.dir);
    if($scope.dir == 0  && $scope.p.step_count > 0){
      //console.log("Changing Polarity");
      //console.log("value before polarity "+$scope.p.step_count);
      $scope.p.step_count = $scope.p.step_count * -1;
    }
    if($scope.dir == 1){
      $scope.p.step_count = Math.abs($scope.p.step_count);
    }
    if($scope.mode == "step"){
      //console.log('Sending Step Data');
      //console.log({"drive_id":1, "position":$scope.p.step_count});
      socket.emit("step_mode",{"drive_id":1, "position":$scope.p.step_count});
    }else{
      //console.log("Sending Jog Mode");
      if($scope.display_text == "START JOG"){
        if($scope.p.feed_rate > 0 && $scope.p.feed_rate < 25){
          //console.log("Starting Jog Now");
          $scope.display_text = "STOP JOG"
          var f = {"dir":$scope.dir,"action":1};
          //console.log(f);
          socket.emit("jog_mode",f);
        }else{
          alert("Invalid Feed rate. Please enter value between 1-20");
        }
      }
      else{
        //console.log("Stopping Jog now");
        socket.emit("jog_mode",{"dir":$scope.dir,"action":0});
        $scope.display_text = "START JOG";
      }
    }
  }
})

.controller('SettingsCtrl', function($scope, $state) {
  $scope.p= {}
  $scope.p.passwd = "";


  $scope.keyboard_options = {numberPad:true,showInMobile:true,
                            forcePosition:'bottom',customClass:'kb_style'};
  $scope.get_access = function(passwd){
    //alert(passwd);
    if(passwd == "1986"){
      $scope.p.passwd = null;
      document.getElementById('passwd').value = "";
      $state.go("app.settings_menu");
    }else{
      $scope.p.passwd = null;
    }
  }
  $scope.back = function(){
    $scope.passwd = "";
    window.history.back();
  }
})
.controller('AutoCtrl', function($scope, $stateParams, $ionicModal, $state, socket, $http) {
  $scope.lineno = "";
  $scope.prev_line = -1;
  $scope.reportEvent = function(evt, obj_id)  {
    console.log(evt);
    console.log(obj_id);
    console.log('Reporting : ' + event.type);
  }
  $scope.selected_line = function(idx){
    console.log(idx);
    if($scope.prev_line >= 0){
      document.getElementById('num'+$scope.prev_line).style.background = "rgb(31, 35, 58)";
    }
    $scope.lineno = idx+1;
    $scope.prev_line = idx;
    document.getElementById('num'+idx).style.background = "rgba(45, 157, 204, 0.75)";
  }
  //console.log($stateParams);
  $scope.display = true;
  //$scope.mode="single";
  //$state.go("app.auto", {'file_name':tmp_prog_name, contents:tmp_prog_contents});
  $scope.mode="1";
  $scope.s_mode_button = "mode_button";
  $scope.c_mode_button = "mode_button_active";
  $scope.execNextLine = function(){
  //console.log("Received next line command.");
  socket.emit("exec_next_line", {"status":"1"});
  }

  $scope.stop_execution = function(){
    //console.log("Inside Stop Execution. Will stop executions now..");
    socket.emit("stop_execution", {"status":"stop"});
  }

  $http.get(DAC_SETTINGS_API)
     .success(function(data){
       //console.log(data);
       //alert("Settings fetch Complete");
       ecs = data.resp.X.ecs;
       //console.log("ECS value is "+ecs);
       if(ecs == 1){
         //Disable Continuous block
         $scope.display = false;
         $scope.mode="0";
         $scope.s_mode_button = "mode_button_active";
         $scope.c_mode_button = "mode_button";
         console.log("Emitting MEssage to server");
         socket.emit('set_program_mode',{mode:'continuous'})
       }else {
         //Enable both
         $scope.display = true;
       }
     })
   .error(function(err){
     //console.log(err);
   })

  $scope.r = Math.random();
  //console.log("Inside auto Controller");
  $scope.file_name = null;
  $scope.contents = [];

  //console.log("storage values",window.localStorage.getItem("file_content"));
  $scope.file_name = window.localStorage.getItem("file_name");
  var fc = window.localStorage.getItem("file_content");
  if(fc !== null && fc !== undefined && fc !== ""){
    $scope.contents = window.localStorage.getItem("file_content").split(",");
  }


  $scope.choose_file = function(){
    $state.go("app.program");
  }

  $scope.setPgmMode = function(mode){
    console.log("Inside Set P");
    if(mode == "s"){
      document.getElementById("ecsbutton").style.visibility = 'visible';
      ecs=1;
      //console.log("Setting ecs to high: "+ecs);
      //console.log("Entered Single block");
      $scope.mode="0";
      $scope.s_mode_button = "mode_button_active";
      $scope.c_mode_button = "mode_button";
      socket.emit("set_program_mode", {mode:"single"});
    }
    else{
      ecs = 0;
      //console.log("Setting ecs to low: "+ecs);
      document.getElementById("ecsbutton").style.visibility = 'hidden';
      //console.log("Entered Continuous block");
      $scope.mode="1";
      $scope.s_mode_button = "mode_button";
      $scope.c_mode_button = "mode_button_active";
      socket.emit("set_program_mode", {mode:"continuous"})
    }
  }

  $scope.startExecution = function(){
    if(ecs == 0)
    document.getElementById("ecsbutton").style.visibility = 'hidden';
    //console.log("Inside Start excution");
    if($scope.file_name == ""){
      alert("Please Select Program");
    }
    if($scope.mode === "" || $scope.mode === undefined){
      alert("Please select Mode");
    }
    //console.log("ECS is "+ecs);
    // if(ecs == 0)
    // document.getElementById("ecsbutton").style.visibility = 'hidden';
    // if($scope.mode == "0")
    // ecs=1;
    //socket.emit("execute", {file_name:$scope.program.name, mode:$scope.program.mode, ecs:$scope.program.ecs});
    /*if($scope.program.ecs == true){
      ecs = 1;
    }*/
    //console.log({file_name:$scope.file_name, mode:$scope.mode, ecs:ecs});
    var line_no = ($scope.lineno >= 0)? $scope.lineno-1 : 0
    socket.emit("execute", {file_name:$scope.file_name, mode:$scope.mode, ecs:ecs, lineno: line_no});
    $scope.lineno = 0;
    ////console.log({file_name:$scope.program.name, mode:$scope.program.mode, ecs:ecs});
  }

  $scope.stopExecution = function(){
    //console.log("Inside Stop Execution. Will stop executions now..");
    socket.emit("stop_execution", {"status":"stop"});

  }



})

.controller('SettingsMenuCtrl', function($scope, $http, $state){
  //console.log("Inside settings ctrl");
  $scope.keyboard_options = {numberPad:true,showInMobile:true,
                            forcePosition:'bottom',customClass:'kb_style'};

  $scope.goto_dac = function(){
    $state.go("app.drive_params");
  }
  $scope.goto_pitch_error = function(){
    $state.go("app.pitch_error");
  }
})

.controller('DriveParamsCtrl', function($scope, $http, $state, socket, $ionicPopup){
  //console.log("Inside DriveParamsCtrl");
  $scope.params = {};
  $scope.tmp_motor_dir = " ";
  $scope.tmp_home_dir = " ";
  $scope.message = "";
  $scope.keyboard_options = {numberPad:true,showInMobile:true,
                            forcePosition:'top',customClass:'kb_style'};
  $scope.toggle = function(val){

    $scope.params[val] = $scope.params[val] == 1? 0 : 1;
    if(val == "ecs"){
        if($scope.params[val] == 1){
          //Enable ECS
          socket.emit("enable_ecs",{"status":"1"});
        }else{
          //Disable ECS
          socket.emit("disable_ecs",{"status":"1"});
        }
    }
    if(val == 'home_dir'){
      $scope.tmp_home_dir = $scope.params[val] == 1? '+VE' : '-VE';
    }
    if(val == 'motor_dir'){
      $scope.tmp_motor_dir = $scope.params[val] == 1? '+VE' : '-VE';
    }
    if(val == 'fin_signal'){
      $scope.tmp_fin_signal = $scope.params[val] == 1? '+VE' : '-VE';
    }
  }

  $scope.whatClassIsIt = function(c){
    //console.log("Classing for --> "+c);
    if($scope.params[c] == 1){
      return 'col dac-buttons selected-color';
    }
    return 'col dac-buttons';
  }
  $http.get(DAC_SETTINGS_API)
   .success(function(data){
     //console.log(data);
     $scope.params = data.resp.X;
     if($scope.params.home_dir == 1){
       $scope.tmp_home_dir = "+VE";
     }else{
       $scope.tmp_home_dir = "-VE";
     }

     if($scope.params.motor_dir == 1){
       $scope.tmp_motor_dir = "+VE";
     }else{
       $scope.tmp_motor_dir = "-VE";
     }

     if($scope.params.fin_signal == 1){
       $scope.tmp_fin_signal = "+VE";
     }else{
       $scope.tmp_fin_signal = "-VE";
     }
   })
   .error(function(err){
     //console.log(err);
   })


   $scope.resetMultiTurnData = function(){
     socket.emit('resetMultiTurn', {status:'1'});
   }

   $scope.udpate_settings = function(){
     //console.log("Inside update update settings --> ");
     var params = {"X":$scope.params};
     //console.log(params);
     $http.post(DAC_SETTINGS_API, params)
      .success(function(data){
        socket.emit('updateSettings',{status:'1'})
        $scope.message = "Setting updated successfully";
        //alert("Settings saved successfully");
        var confirmPopup = $ionicPopup.show({
         title: 'Settings Saved',
         template: '<h2 style="color:black;"> Settings Params Saved Successfully</h2>',
         scope:$scope,
         buttons: [
           { text: 'OK', type: 'button-positive' }
         ]
       });

       confirmPopup.then(function(res) {
         if(res) {
           console.log('You are sure');
         } else {
           console.log('You are not sure');
         }
       });




      })
      .error(function(err){
        $scope.message = "Failed to update settings. Please try again.";
      })
   }

   $scope.back = function(){
     $state.go('app.home')
   }
})

.controller('PitchErrorCtrl', function($scope, $http, $state){
  $scope.params = [];
  $scope.params1 = [];
  $scope.params2 = [];
  $scope.params3 = [];
  $scope.params4 = [];
  $scope.rp = {};
  $scope.p = {};
  $scope.keyboard_options = {numberPad:true};
  $http.get(DAC_SETTINGS_API)
   .success(function(data){
     //console.log(data);
     $scope.rp = data.resp;
     $scope.params = data.resp.X.pitch_error;
     //console.log($scope.params);
     $scope.params1 = $scope.params.splice(0,9);
     //console.log($scope.params1);
     $scope.params2 = $scope.params.splice(0,9);
     //console.log($scope.params2);
     $scope.params3 = $scope.params.splice(0,9);
     //console.log($scope.params3);
     $scope.params4 = $scope.params;
     //console.log($scope.params4);
   })
   .error(function(err){
     //console.log(err);
   });

   $scope.update_pe = function(){
     //console.log($scope.p);
     var arr = [];
     for(var i=0; i<36; i++){
       var obj = document.getElementById(i);
       /*if(obj.value > 0.1){
         alert('Pitch error value cannot be greater than 0.1');
         return;
       }*/

       if(isNaN(obj.value)){
         alert('Values should be numerical');
         return;
       }
       arr.push(obj.value);
     }
     arr[35] = 0;
     $scope.rp.X.pitch_error = arr;
     $http.post(DAC_SETTINGS_API, $scope.rp)
      .success(function(data){
        $scope.message = "Setting updated successfully";
      })
      .error(function(err){
        $scope.message = "Failed to update settings. Please try again.";
      })
   }

   $scope.back = function(){
     $state.go("app.home");
   }
})

.controller('programCtrl', function($scope, $http, $ionicModal, $state) {
  //$scope.array = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17];
  $scope.array = [];
  $scope.file_contents = [];
  $scope.p= {code_area : ''};
  $scope.selected_file = "";
  $scope.keyboard_options = {numberPad:true,showInMobile:true,forcePosition:'bottom',customClass:'kb_style'};
  $ionicModal.fromTemplateUrl('program_contents.html', {
    scope: $scope,
    animation: 'slide-in-up'
  }).then(function(modal) {
    $scope.modal = modal;
    //$scope.modal.show();
  });
  $scope.number = 5;
  $scope.getNumber = function(num) {
    return new Array(num);
  }
  $scope.openModal = function() {
    $scope.modal.show();
    $scope.p.message = "";
  };
  $scope.closeModal = function() {
    $scope.modal.hide();
  };
  // Cleanup the modal when we're done with it!
  $scope.$on('$destroy', function() {
    $scope.modal.remove();
  });

  $ionicModal.fromTemplateUrl('new_program.html', {
    scope: $scope,
    animation: 'slide-in-up'
  }).then(function(modal) {
    $scope.modal1 = modal;
  });

  $scope.openModal1 = function() {
    $scope.modal1.show();
    $scope.p.message = "";
  };
  $scope.closeModal1 = function() {
    $scope.modal1.hide();
  };
  // Cleanup the modal when we're done with it!
  $scope.$on('$destroy', function() {
    $scope.modal1.remove();
  });

  $scope.new_program = function(){
    $scope.openModal1();
  }

  $scope.getPrograms = function(){
    $http.get(GET_PROGRAMS_API)
     .success(function(data){
       //console.log(data);
       $scope.array = data.files;
     })
     .error(function(err){
       //console.log(err);
     })
  }
  $scope.getPrograms();
   $scope.openFile = function(idx){
     //alert(idx);
     $scope.selected_file = $scope.array[idx];
     $scope.openModal();
     $scope.p.name = $scope.selected_file;
     $http.get(FILE_CONTENT_API+"?file_name="+$scope.array[idx])
     .success(function(data){
       //socket.emit('execute', {file:file_name, mode:$scope.program.mode});
       //console.log(data.contents);
       $scope.file_contents = []
       $scope.file_data = data.contents;
       var file_contents = $scope.file_data.split("<br>");
       for(var i=0; i<file_contents.length; i++){
         if(file_contents[i] !== ""){
           $scope.file_contents.push(file_contents[i]);
           ////console.log(file_contents[i]);
         }
       }
       $scope.file_contents = $scope.file_contents;
       //console.log("File Contents", $scope.file_contents);
       //console.log($scope.p.code_area);
       $scope.p.code_area = $scope.file_contents.join("\n");
       //console.log($scope.p.code_area);
     })
     .error(function(err){
       //console.log("Error access get api", err);
     });
   }

   $scope.save_program = function(){
     //console.log($scope.p.code_area);
     var resp = validateProgram($scope.p.code_area);
     //console.log(resp);
     if(resp.status == "error"){
       //console.log("Error received");
       $scope.message = "Invalid syntax at line number "+resp.line;
       return;
     }
     $http.post(CREATE_PROGRAM_API, {file_name:$scope.selected_file, contents:$scope.p.code_area})
       .success(function(data){
         //console.log("File created succesfully");
         $scope.message  = "Program Saved Successfully";
         $scope.file_contents = $scope.p.code_area.split("\n");
         //$scope.getPrograms();
       })
       .error(function(data){
         //console.log("Program Creation Failed. Please re-try.");
         //console.log("Failed to save Program");
       });
   }

   $scope.new_program = function(){

     $scope.max_rpm = 20;
     $http.get(DAC_SETTINGS_API)
        .success(function(data){
          //console.log(data);
          //alert("Settings fetch Complete");
          $scope.max_rpm = data.resp.X.max_rpm;
        })
      .error(function(err){
        //console.log(err);
      })

     //console.log($scope.p.prgm);
     //console.log($scope.p.pgm_code_area);
     if($scope.p.pgm_code_area === undefined){
       alert("File contents empty");
       return;
     }
     if($scope.p.prgm === undefined){
       alert("Please enter file name");
       return;
     }
     //console.log("Params are");
     //console.log({file_name:$scope.p.prgm, contents:$scope.p.pgm_code_area});
     var resp = validateProgram($scope.p.pgm_code_area);
     //console.log(resp);
     if(resp.status == "error"){
       //console.log("Error received");
       $scope.message = "Invalid syntax at line number "+resp.line;
       return;
     }

     var reg = /f(\d+)/gi;
     var fr = $scope.p.pgm_code_area.match(reg);
     if(fr !== null && fr !== undefined){
       for(var i=0; i<fr.length; i++){
         var rpm = fr[i].slice(1, fr[i].length);
         console.log($scope.max_rpm);
         if(rpm > $scope.max_rpm){
           $scope.message = "Max supported RPM is "+$scope.max_rpm+", at "+fr[i];
           return;
         }
       }
     }

     $http.post(CREATE_PROGRAM_API, {file_name:$scope.p.prgm, contents:$scope.p.pgm_code_area})
       .success(function(data){
         //console.log(data);
         //console.log("File created succesfully");
         $scope.message  = "Program Created Successfully";
         $scope.getPrograms();
         setTimeout(function(){
           $scope.p.prgm = "";
           $scope.p.pgm_code_area = "";
           $scope.closeModal1();
         },3000)
       })
       .error(function(data){
         //console.log("Program Creation Failed. Please re-try.");
         //console.log("Failed to save Program");
       });
   }

   $scope.rename_program = function(){
     var new_name = $scope.p.name;
     $http.get(RENAME_API+"?file_name="+$scope.selected_file+"&new_file_name="+new_name)
      .success(function(data){
        $scope.message = "File renamed successfully";
        $scope.getPrograms();
      })
      .error(function(err){
        $scope.message = "Raname Failed";
      })
   }
   $scope.delete_program = function(){
     $http.get(DELETE_API+"?file_name="+$scope.selected_file)
      .success(function(data){
        $scope.message = "File removed successfully";
        setTimeout(function(){
          $scope.getPrograms();
          $scope.closeModal();
        }, 2000);
      })
      .error(function(err){
        //console.log(err);
        $scope.message = "Failed to remove file";
      })
   }

   $scope.execute_program = function(){
     //console.log("Will execute now");
    window.localStorage.setItem("file_name",$scope.selected_file);
    //console.log("inserting content",$scope.file_contents);
    window.localStorage.setItem("file_content",$scope.file_contents);
    $state.go("app.auto", {'file_name':$scope.selected_file, contents:$scope.file_contents});
    $scope.closeModal();
   }



});
