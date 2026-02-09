
import './notebook-cell-renderer.css';

import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import classNames from "classnames";
import VeloAce, { SettingsButton } from '../core/ace.jsx';
import NotebookReportRenderer from './notebook-report-renderer.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Dropdown from 'react-bootstrap/Dropdown';
import FormControl from 'react-bootstrap/FormControl';
import Navbar from 'react-bootstrap/Navbar';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import CreateArtifactFromCell from './create-artifact-from-cell.jsx';
import AddCellFromFlowDialog from './add-cell-from-flow.jsx';
import Completer from '../artifacts/syntax.jsx';
import VeloTimestamp from "../utils/time.jsx";
import { AddTimelineDialog, AddVQLCellToTimeline } from "./timelines.jsx";
import T from '../i8n/i8n.jsx';
import ViewCellLogs from "./logs.jsx";
import CopyCellToNotebookDialog from './notebook-copy-cell.jsx';
import FormatTableDialog from './notebook-format-tables.jsx';
import NotebookUploads from '../notebooks/notebook-uploads.jsx';
import ToolTip from '../widgets/tooltip.jsx';

import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import VeloTable, { getFormatter } from '../core/table.jsx';


const cell_types = ["Markdown", "VQL"];

class AddCellFromHunt extends React.PureComponent {
    static propTypes = {
        closeDialog: PropTypes.func.isRequired,

        // func(text, type, env)
        addCell: PropTypes.func,
    }

    addCellFromHunt = (hunt) => {
        var hunt_id = hunt["hunt_id"];
        var query = "SELECT * \nFROM hunt_results(\n";
        var sources = hunt["artifact_sources"] || hunt["start_request"]["artifacts"];
        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    // artifact='" + sources[i] + "',\n";
        }
        query += "    hunt_id='" + hunt_id + "')\nLIMIT 50\n";

        this.props.addCell(query, "VQL");
        this.props.closeDialog();
    }

    state = {
        hunts: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        api.get("v1/ListHunts", {
            count: 100,
            offset: 0,
        }, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let hunts = response.data.items || [];
            this.setState({hunts: hunts});
        });
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    render() {
        let columns = ["state", "hunt_id", "hunt_description",
                       "create_time", "start_time", "expires",
                       "total_clients_scheduled", "creator"];
        let header_renderers = {
            state:  T("State"),
            hunt_id: T("Hunt ID"),
            hunt_description: T("Description"),
            create_time: T("Created"),
            start_time: T("Started"),
            expires: T("Expires"),
            total_clients_scheduled: T("Scheduled"),
            creator: T("Creator"),
        };

        let column_renderers = {
            expires: getFormatter("timestamp"),
            state: (cell, row) => {
                let stopped = row.stats && row.stats.stopped;
                if (stopped || cell === "STOPPED") {
                    return <div className="hunt-status-icon">
                             <FontAwesomeIcon icon="stop" /></div>;
                }
                if (cell === "RUNNING") {
                    return <div className="hunt-status-icon">
                             <FontAwesomeIcon icon="hourglass" /></div>;
                }
                if (cell === "PAUSED") {
                    return <div className="hunt-status-icon">
                             <FontAwesomeIcon icon="pause" /></div>;
                }
                return <div className="hunt-status-icon">
                         <FontAwesomeIcon icon="exclamation" /></div>;
            },
            create_time: getFormatter("timestamp"),
            start_time: getFormatter("timestamp"),
            total_clients_scheduled: (cell, row) => {
                return row.stats && row.stats.total_clients_scheduled;
            },
        };

        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Add cell from Hunt")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <div className="no-margins selectable">
                  { _.isEmpty(this.state.hunts) ?
                    <div className="no-content">
                      {T("No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.")}
                    </div>
                    :
                    <VeloTable
                      rows={this.state.hunts}
                      columns={columns}
                      column_renderers={column_renderers}
                      header_renderers={header_renderers}
                      onSelect={(row, idx)=>{
                          this.addCellFromHunt(row);
                      }}
                    />
                  }
                </div>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.addCellFromHunt}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class NotebookCellRenderer extends React.Component {
    static propTypes = {
        env: PropTypes.object,
        cell_metadata: PropTypes.object,
        notebook_id: PropTypes.string,
        notebook_metadata: PropTypes.object.isRequired,
        selected_cell_id: PropTypes.string,
        setSelectedCellId: PropTypes.func,

        // Manage notebook lock state. The notebook tries to avoid
        // confusing by locking updates while any cell is calculating.
        notebookLocked: PropTypes.number,
        incNotebookLocked: PropTypes.func,

        upCell: PropTypes.func,
        downCell: PropTypes.func,
        deleteCell: PropTypes.func,

        // func(notebook_cell_id, text, type, env)
        addCell: PropTypes.func,

        // Causes the notebooks to be refreshed
        updateVersion: PropTypes.func,
    };

    state = {
        // The state of the cell is determined by both its ID and its
        // last modified timestamp. As the table is refreshed, the
        // server will advance the timestamp causing a refresh.
        cell_timestamp: 0,
        cell_id: "",

        // The full cell data.
        cell: {},

        // The ace editor.
        ace: {},

        currently_editing: false,
        input: "",

        showAddTimeline: false,
        showAddCellToTimeline: false,
        showAddCellFromHunt: false,
        showAddCellFromFlow: false,
        showCreateArtifactFromCell: false,
        showCopyCellToNotebook: false,
        showSuggestionSubmenu: false,
        showMoreLogs: false,
        showFormatTablesDialog: false,
        showNotebookUploadsDialog: false,

        local_completions_lookup: {},
        local_completions: [],

        // Only load the cell the first time if it is visible.
        visible: false,
        unloaded: true,
    }

    constructor(props) {
        super(props);
        this.myRef = React.createRef();
        this.scrollRef = React.createRef();
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.update_source = CancelToken.source();

        // Install an observer to figure out when the cell is visible.
        this.observer = new IntersectionObserver(x=>{
            if(x) {
                this.setState({visible: x[0].isIntersecting});
            }
        });
        this.observer.observe(this.myRef.current, );

        this.fetchCellContents();
    }

    componentWillUnmount() {
        this.source.cancel();
        this.update_source.cancel();

        if (this.observer) {
            this.observer.disconnect();
        };
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        // Do not update the editor if we are currently editing it -
        // otherwise it will wipe the text.
        let current_cell_id = this.state.cell_id;
        let selected = current_cell_id === this.props.selected_cell_id;
        if (!selected && this.state.currently_editing) {
            // We are not currently editing if this cell is not selected.
            this.setState({currently_editing: false});
            return;
        }

        // We detect the cell has changed by looking at both cell id
        // and the cell timestamp.
        let props_cell_timestamp = this.props.cell_metadata &&
            this.props.cell_metadata.timestamp;
        let props_cell_id = this.props.cell_metadata &&
            this.props.cell_metadata.cell_id;

        if (prevProps.notebook_id !== this.props.notebook_id ||
            props_cell_timestamp > this.state.cell_timestamp ||
            prevState.visible != this.state.visible ||
            props_cell_id !== current_cell_id) {

            // Prevent further updates to this cell by setting the
            // cell id and timestamp.
            this.setState({cell_timestamp: props_cell_timestamp,
                           cell_id: props_cell_id,
                           });
            this.fetchCellContents();
        }
    };

    registerCompletions = (completions)=>{
        let local_completions = this.state.local_completions;
        let local_completions_lookup = this.state.local_completions_lookup;
        _.map(completions, x=>{
            if(!local_completions_lookup[x]) {
                local_completions_lookup[x] = true;
                local_completions.push(x);
            }
        });
        this.setState({
            local_completions_lookup: local_completions_lookup,
            local_completions: local_completions});
    }

    getEnv = ()=>{
        let env = Object.assign({},this.props.env || {});
        return Object.assign(env, {
            notebook_id: this.props.notebook_id,
            notebook_cell_id: this.props.cell_metadata && this.props.cell_metadata.cell_id});
    }

    fetchCellContents = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        let cell_version = this.props.cell_metadata &&
            this.props.cell_metadata.current_version;

        if (!this.state.visible && this.state.unloaded) {
            let cell = this.state.cell;
            cell.cell_id = this.props.cell_metadata.cell_id;

            // Take up a reasonable amount of vertical space to keep
            // further cells invisible.
            cell.output = "<div class='cell-placeholder'>" + T("Loading") + "</div>";
            this.setState({cell: cell});
            return;
        }

        this.props.incNotebookLocked(1);
        api.get("v1/GetNotebookCell", {
            notebook_id: this.props.notebook_id,
            cell_id: this.props.cell_metadata.cell_id,
            version: cell_version,
        }, this.source.token).then((response) => {
            this.props.incNotebookLocked(-1);

            if (response.cancel) {
                return;
            }

            let cell = response.data;
            if (!this.state.currently_editing) {
                this.setState({cell: cell, unloaded: false,
                               input: cell.input,
                               loading: false});
            }


        }).catch((e) => {
            this.props.incNotebookLocked(-1);

            let message = e.response && e.response.data &&
                e.response.data.message;
            let cell = Object.assign({}, this.props.cell_metadata || {});
            cell.messages = [message];
            this.setState({loading: false, unloaded: false, cell: cell});
        });;
    };

    setEditing = (edit) => {
        this.setState({currently_editing: edit});
    };

    getPlaceholder = () => {
        let type = this.ace_type(this.state.cell && this.state.cell.type);
        if (type === "vql") {
            return T("Type VQL to evaluate on the server (press ? for help)");
        };
        if (type === "markdown") {
            return T("Enter markdown text to render in the notebook");
        };
        return type;
    }

    aceConfig = (ace) => {
        // Attach a completer to ACE.
        let completer = new Completer();
        completer.initializeAceEditor(ace, {});
        completer.registerCompletions(this.state.local_completions);
        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25,
            placeholder: this.getPlaceholder(),
        });

        this.setState({ace: ace, completer: completer});
    };

    ace_type = (type) => {
        if (!type) {
            type = "vql";
        }

        if (type.toLowerCase() === "vql") {
            return "vql";
        }
        if (type.toLowerCase() === "markdown") {
            return "markdown";
        }
        if (type.toLowerCase() === "artifact") {
            return "yaml";
        }

        return "yaml";
    }

    recalculate = (cell) => {
        cell.output = T("Loading");
        cell.timestamp = 0;
        cell.calculating = true;
        cell.messages = [];

        this.setState({cell: cell,
                       local_completions: [],
                       loading: true,
                       local_completions_lookup: {},
                      });

        // Reset any inflight calls.
        this.update_source.cancel();
        this.update_source = CancelToken.source();

        // Lock the notebook until we refresh the notebook.
        this.props.incNotebookLocked(1);

        api.post('v1/UpdateNotebookCell', {
            notebook_id: this.props.notebook_id,
            cell_id: this.state.cell.cell_id,
            type: this.state.cell.type || "Markdown",
            env: this.state.cell.env,
            currently_editing: false,
            input: this.state.cell.input,
        }, this.update_source.token).then( (response) => {
            this.props.incNotebookLocked(-1);

            if (response.cancel) {
                this.setState({currently_editing: false});
                return;
            }

            let keep_editing = false;
            if (response.data && response.data.error) {
                // Send a snackbar error as well.
                api.error(response.data.error);
                keep_editing = true;
            }

            let cell = response.data;
            if (this.props.cell_metadata.timestamp > cell.timestamp) {
                // The existing cell is actually newer - this can
                // happen if the periodic update raced
                // UpdateNotebookCell and got a newer version.
            } else if (cell.cell_id === this.props.cell_metadata.cell_id) {
                this.setState({cell: response.data,
                               loading: false,
                               currently_editing: keep_editing});
            } else {
                this.fetchCellContents();
            }

        }).catch(e=>{
            this.props.incNotebookLocked(-1);
        });

    }

    formatCell = () => {
        api.post('v1/ReformatVQL', {
            vql:  this.state.ace.getValue(),
        }, this.source.token).then( (response) => {
            if (response.cancel) {
                return;
            }

            let reformatted = response.data && response.data.vql;
            if (reformatted) {
                this.state.ace.setValue(reformatted);
            }
        });
    }

    saveCell = (cell) => {
        cell.output = T("Loading");
        cell.timestamp = 0;
        cell.calculating = true;
        cell.messages = [];

        // Clear completions as they will be updated once the cell is
        // re-calculated.
        this.setState({cell: cell,
                       local_completions: [],
                       currently_editing: false,
                       local_completions_lookup: {},
                      });

        // Reset any inflight calls.
        this.update_source.cancel();
        this.update_source = CancelToken.source();

        this.props.incNotebookLocked(1);

        api.post('v1/UpdateNotebookCell', {
            notebook_id: this.props.notebook_id,
            cell_id: cell.cell_id,
            type: cell.type || T("Markdown"),
            env: this.state.cell.env,
            currently_editing: false,
            input: cell.input,
        }, this.update_source.token).then( (response) => {
            this.props.incNotebookLocked(-1);

            if (response.cancel) {
                this.setState({currently_editing: false});
                return;
            }

            let keep_editing = false;
            if (response.data && response.data.error) {
                // Send a snackbar error as well.
                api.error(response.data.error);
                keep_editing = true;
            }

            let cell = response.data;
            if (cell.cell_id === this.props.cell_metadata.cell_id) {
                this.setState({cell: response.data,
                               loading: false,
                               currently_editing: keep_editing});

                // Update the ACL edit box if needed.
                if (this.state.ace &&
                    this.state.ace.setValue &&
                    response.data.input) {

                    this.state.ace.setValue(response.data.input);
                }

            };
        }).catch(response=>{
            this.props.updateVersion();
        });
    };

    deleteCell = () => {
        this.props.deleteCell(this.state.cell.cell_id);
        this.setState({currently_editing: false});
    }

    stopCalculating = () => {
        api.post("v1/CancelNotebookCell", {
            notebook_id: this.props.notebook_id,
            cell_id: this.state.cell.cell_id,
            version: this.state.cell.current_version,
        }, this.source.token).then(response=>{
            if (response.cancel) {
                return;
            }

            // Refresh the notebook with the current cell version.
            this.props.updateVersion();
        });
    }

    pasteEvent = (e) => {
        let items = e && e.clipboardData && e.clipboardData.items;
        if (!items) {return;}

        for (var i = 0; i < items.length; i++) {
            let item = items[i];

            if (item.kind === 'file') {
                let blob = item.getAsFile();
                let reader = new FileReader();
                let current_cell_id = this.state.cell_id;

                reader.onload = (event) => {
                    let request = {
                        data: reader.result.split(",")[1],
                        notebook_id: this.props.notebook_id,
                        cell_id: current_cell_id,
                        filename: blob.name,
                        size: blob.size,
                    };

                    api.post(
                        'v1/UploadNotebookAttachment', request, this.source.token
                    ).then((response) => {
                        if (response.cancel) return;

                        // Add direct html to ensure exports recognize
                        // it.
                        let filename = encodeURI(blob.name);
                        let url = encodeURI(response.data.url);
                        let mime_type = response.data && response.data.mime_type;
                        if (/image/.test(mime_type || "")) {
                            this.state.ace.insert(
                                "\n<img src=\"" +
                                    url + "\" alt=\"" +
                                    filename + "\"/>\n");
                        } else {
                            this.state.ace.insert(
                                "\n<a href=\"" +
                                    url + "\">" +
                                    filename + "</a>\n");
                        }

                    }, function failure(response) {
                        console.log("Error " + response.data);
                    });
                };
                reader.readAsDataURL(blob);
            }
        }
    };

    addCellFromCell = () => {
        // Try to figure out the  tables shown in this cell.
        var myRegexp = /"table_id":([0-9]+)/g;
        let match = myRegexp.exec(this.state.cell.output);
        let tables = [];
        while (match != null) {
            // matched text: match[0]
            // match start: match.index
            // capturing group n: match[n]
            tables.push(match[1]);
            match = myRegexp.exec(this.state.cell.output);
        }

        let content = "SELECT *\nFROM source(\n  notebook_id=\"" +
            this.props.notebook_id + "\",\n";
        for(let i=0; i<tables.length;i++) {
            if(i===0) {
                content += "  notebook_cell_table=" + tables[i]+ ",\n";
            } else {
                content += "--  notebook_cell_table=" + tables[i]+ ",\n";
            }
        }

        content += "  notebook_cell_id=\""+ this.state.cell.cell_id +
            "\")\nLIMIT 50\n";

        this.props.addCell(this.state.cell.cell_id, "VQL", content,
                           this.state.cell.env);
    }

    showSuggestions = ()=>{
        let suggestions = this.props.notebook_metadata &&
            this.props.notebook_metadata.suggestions;
        if (!suggestions) {
            return <></>;
        }

        return <>
                 <Dropdown.Menu>
                     { _.map(suggestions, (x, i)=>{
                         return <Dropdown.Item
                                  key={i}

                                  onClick={()=>{
                                      this.props.addCell(
                                          this.state.cell.cell_id,
                                          x.type,
                                          x.input,
                                          x.env);
                                  }}
                                  title={x.name}>
                                  {x.name}
                                </Dropdown.Item>;
                     })}
                   </Dropdown.Menu>
               </>;
    }

    undo_available = ()=>{
        if(!this.state.cell || this.state.loading) {
            return false;
        }

        let available_versions = this.state.cell.available_versions || [];
        let current_version = this.state.cell.current_version;

        return available_versions.length > 0 && current_version !== available_versions[0];
    }

    redo_available = ()=>{
        if(!this.state.cell || this.state.loading) {
            return false;
        }

        let available_versions = this.state.cell.available_versions || [];
        let current_version = this.state.cell.current_version;

        return available_versions.length > 0 &&
            current_version !== available_versions[available_versions.length-1];
    }

    undo = ()=>{
        if(!this.state.cell) {
            return;
        }

        let available_versions = this.state.cell.available_versions || [];
        let current_version = this.state.cell.current_version;

        for(let i=0; i<available_versions.length; i++) {
            if (available_versions[i]===current_version && i > 0) {
                this.setState({loading: true});

                api.post("v1/RevertNotebookCell", {
                    notebook_id: this.props.notebook_id,
                    cell_id: this.state.cell.cell_id,
                    version: available_versions[i-1],
                }, this.source.token).then(response=>{
                    if (response.cancel) {
                        return;
                    }
                    let cell = response.data;
                    this.setState({cell: cell,
                                   input: cell.input,
                                   loading: false});
                });

                return;
            }
        };
    }

    redo = ()=>{
        if(!this.state.cell) {
            return;
        }

        let available_versions = this.state.cell.available_versions || [];
        let current_version = this.state.cell.current_version;

        for(let i=0; i<available_versions.length; i++) {
            if (available_versions[i]===current_version &&
                i < available_versions.length) {
                this.setState({loading: true});

                api.post("v1/RevertNotebookCell", {
                    notebook_id: this.props.notebook_id,
                    cell_id: this.state.cell.cell_id,
                    version: available_versions[i+1],
                }, this.source.token).then(response=>{
                    if (response.cancel) {
                        return;
                    }
                    let cell = response.data;
                    this.setState({cell: cell,
                                   input: cell.input,
                                   loading: false});
                });

                return;
            }
        };
    }

    render() {
        let selected = this.props.selected_cell_id &&
            this.state.cell.cell_id === this.props.selected_cell_id;

        let cell_id = this.state.cell.cell_id;
        let cells = (this.props.notebook_metadata &&
                     this.props.notebook_metadata.cell_metadata) || [];
        let pos = 0;
        // Find the cell position.
        for(pos=0;pos < cells.length; pos++) {
            if(cell_id == cells[pos].cell_id) {
                break;
            }
        }

        // If the cell is first it can not go up.
        let cellCanGoUp = pos !== 0;

        // If the cell is last it can not go down.
        let cellCanGoDown = pos < cells.length -1 ;

        // There are 3 states for the cell:
        // 1. The cell is selected but not being edited: Show the cell manipulation toolbar.
        // 2. The cell is being edited: Show the editing toolbar
        // 3. The cell is not selected or edited - no decorations.
        let non_editing_toolbar = (
            <>
              <ButtonGroup>
                <ToolTip tooltip={T("Cancel")}>
                  <Button onClick={() => {this.props.setSelectedCellId("");}}
                          variant="default">
                    <FontAwesomeIcon icon="window-close"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Recalculate")}>
                  <Button disabled={this.state.cell.calculating}
                          onClick={()=>this.recalculate(this.state.cell)}
                          variant="default">
                    <FontAwesomeIcon icon="sync"/>
                  </Button>
                </ToolTip>

                <ToolTip tooltip={T("Stop Calculating")}>
                  <Button disabled={!this.state.cell.calculating ||
                                    this.state.loading}
                          onClick={this.stopCalculating}
                          variant="default">
                    <FontAwesomeIcon icon="stop"/>
                  </Button>
                </ToolTip>
                { !this.state.collapsed &&
                  <ToolTip tooltip={T("Collapse")}>
                    <Button onClick={() => this.setState({collapsed: true})}
                            variant="default">
                      <FontAwesomeIcon icon="compress"/>
                    </Button>
                  </ToolTip>
                }
                { this.state.collapsed &&
                  <ToolTip tooltip={T("Expand")}>
                    <Button onClick={() => this.setState({collapsed: false})}
                            variant="default">
                      <FontAwesomeIcon icon="expand"/>
                    </Button>
                  </ToolTip>
                }
                <ToolTip tooltip={T("Edit Cell")}>
                  <Button disabled={this.state.cell.calculating}
                          onClick={() => { this.setEditing(true); }}
                          variant="default">
                    <FontAwesomeIcon icon="pencil-alt"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Up Cell")}>
                  <Button disabled={!cellCanGoUp}
                          onClick={() => {
                              this.props.upCell(this.state.cell.cell_id);
                          }}
                          variant="default">
                    <FontAwesomeIcon icon="arrow-up"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Down Cell")}>
                  <Button disabled={!cellCanGoDown}
                          onClick={() => {
                              this.props.downCell(this.state.cell.cell_id);
                          }}
                          variant="default">
                    <FontAwesomeIcon icon="arrow-down"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Undo")}>
                  <Button disabled={!this.undo_available()}
                          onClick={this.undo}
                          variant="default">
                    <FontAwesomeIcon icon="rotate-left"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Redo")}>
                  <Button disabled={!this.redo_available()}
                          onClick={this.redo}
                          variant="default">
                    <FontAwesomeIcon icon="rotate-right"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Copy Cell")}>
                  <Button onClick={()=>this.setState({showCopyCellToNotebook: true})}
                          variant="default">
                    <FontAwesomeIcon icon="file-import"/>
                  </Button>
                </ToolTip>

                {this.state.cell && this.state.cell.type === "vql" &&
                 <>
                   <ToolTip tooltip={T("Add Timeline")}>
                     <Button onClick={()=>this.setState({showAddCellToTimeline: true})}
                             variant="default">
                       <FontAwesomeIcon icon="calendar-alt"/>
                     </Button>
                   </ToolTip>
                   <ToolTip tooltip={T("Format Columns")}>
                     <Button onClick={()=>this.setState({showFormatTablesDialog: true})}
                             variant="default">
                       <FontAwesomeIcon icon="table"/>
                     </Button>
                   </ToolTip>
                   <ToolTip tooltip={T("Notebook Uploads")}>
                     <Button onClick={() => this.setState({ showNotebookUploadsDialog: true })}
                             variant="default">
                       <FontAwesomeIcon icon="fa-file-download" />
                       <span className="sr-only">{T("Notebook Uploads")}</span>
                     </Button>
                   </ToolTip>
                 </>
                }
                <ToolTip tooltip={T("Add Cell")}>
                  <Dropdown variant="default">
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="plus"/>
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      <Dropdown.Item
                        title="Markdown"
                        onClick={() => {
                            // Preserve the current cell's environemnt for the new cell
                            this.props.addCell(this.state.cell.cell_id, "Markdown", "", this.state.cell.env);
                        }}>
                        Markdown
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="VQL"
                        onClick={() => {
                            this.props.addCell(this.state.cell.cell_id, "VQL", "", this.state.cell.env);
                        }}>
                        VQL
                      </Dropdown.Item>
                      <hr/>

                      <Dropdown
                        title={T("Suggestion")}
                        drop="end"
                        variant="default-outline">
                        <Dropdown.Toggle
                          className="dropdown-item"
                          variant="default-outline">
                          {T("Suggestions")}
                        </Dropdown.Toggle>
                        { this.showSuggestions() }
                      </Dropdown>
                      <hr/>

                      <Dropdown.Item
                        title={T("Add Timeline")}
                        onClick={()=>this.setState({showAddTimeline: true})}>
                        {T("Add Timeline")}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T("Add Cell From This Cell")}
                        onClick={this.addCellFromCell}>
                        {T("Add Cell From This Cell")}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T("Add Cell From Hunt")}
                        onClick={()=>this.setState({showAddCellFromHunt: true})}>
                        {T("Add Cell From Hunt")}
                      </Dropdown.Item>
                      <Dropdown.Item
                        title={T("Add Cell From Flow")}
                        onClick={()=>this.setState({showAddCellFromFlow: true})}>
                        {T("Add Cell From Flow")}
                      </Dropdown.Item>
                      {this.state.cell && this.state.cell.type === "VQL" &&
                       <Dropdown.Item
                         title={T("Create Artifact from VQL")}
                         onClick={()=>this.setState({showCreateArtifactFromCell: true})}>
                         {T("Create Artifact from VQL")}
                       </Dropdown.Item>}
                    </Dropdown.Menu>
                  </Dropdown>
                </ToolTip>
              </ButtonGroup>
              <ButtonGroup className="float-right">
                <ToolTip tooltip={T("Rendered")}>
                  <Button variant="outline-info">
                    <VeloTimestamp usec={this.state.cell.timestamp } />
                    { this.state.cell.duration &&
                      <span>&nbsp;({this.state.cell.duration}s) </span> }
                  </Button>
                </ToolTip>
              </ButtonGroup>
            </>
        );

        let ace_toolbar = (
            <>
              <ButtonGroup>
                <ToolTip tooltip={T("Close")}>
                  <Button onClick={() => {this.setEditing(false); }}
                          variant="default">
                    <FontAwesomeIcon icon="window-close"/>
                  </Button>
                </ToolTip>

                <SettingsButton ace={this.state.ace}/>

                { this.state.cell.type === "vql" &&
                  <ToolTip tooltip={T("Reformat VQL")}>
                    <Button onClick={this.formatCell}
                            variant="default">
                      <FontAwesomeIcon icon="indent"/>
                    </Button>
                  </ToolTip>
                }
                <ToolTip tooltip={T("Save & Run")}>
                  <Button onClick={()=>{
                              let cell = this.state.cell;
                              cell.input = this.state.ace.getValue();
                              this.saveCell(cell);

                              // Do not allow the calculation to be
                              // cancelled until we receive the version
                              // id from the update API.
                              this.setState({loading: true});
                          }}
                          variant="default">
                    <FontAwesomeIcon icon="save"/>
                  </Button>
                </ToolTip>
              </ButtonGroup>

              <ButtonGroup className="float-right">
                <ToolTip tooltip={T("Delete Cell")}>
                  <Button onClick={this.deleteCell}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>
                </ToolTip>

                <FormControl as="select"
                             value={this.state.cell.type}
                             onChange={x=> {
                                 let cell = this.state.cell;
                                 cell.type = x.currentTarget.value;
                                 this.setState({cell: cell});
                             }} >
                  { _.map(cell_types, (v, idx) => {
                      return <option value={v.toLowerCase()} key={idx}>
              {v}
            </option>;
                  })}
                </FormControl>
              </ButtonGroup>
            </>
        );

        return (
            <div ref={this.scrollRef}>
            { this.state.showAddCellFromHunt &&
                <AddCellFromHunt
                  addCell={(text, type, env)=>{
                      this.props.addCell(this.state.cell.cell_id, type, text, env);
                  }}
                  closeDialog={()=>this.setState({showAddCellFromHunt: false})} />
              }
              { this.state.showAddCellFromFlow &&
                <AddCellFromFlowDialog
                  addCell={(text, type, env)=>{
                      this.props.addCell(this.state.cell.cell_id, type, text, env);
                  }}
                  closeDialog={()=>this.setState({showAddCellFromFlow: false})} />
              }
              { this.state.showFormatTablesDialog &&
                <FormatTableDialog
                  cell={this.state.cell}
                  notebook_metadata={this.props.notebook_metadata}
                  saveCell={cell=>{
                      this.saveCell(cell);
                  }}
                  columns={this.state.local_completions}
                  closeDialog={()=>this.setState({showFormatTablesDialog: false})} />
              }
              { this.state.showCreateArtifactFromCell &&
                <CreateArtifactFromCell
                  vql={this.state.input}
                  onClose={()=>this.setState({showCreateArtifactFromCell: false})} />
              }
              { this.state.showAddTimeline &&
                <AddTimelineDialog
                  notebook_metadata={this.props.notebook_metadata}
                  addCell={(text, type, env)=>{
                      this.props.addCell(this.state.cell.cell_id, type, text, env);
                  }}
                  closeDialog={()=>this.setState({showAddTimeline: false})} />
              }
              { this.state.showAddCellToTimeline &&
                <AddVQLCellToTimeline
                  cell={this.state.cell}
                  notebook_metadata={this.props.notebook_metadata}
                  closeDialog={()=>this.setState({showAddCellToTimeline: false})} />
              }
              { this.state.showMoreLogs &&
                <ViewCellLogs
                  cell={this.state.cell}
                  notebook_metadata={this.props.notebook_metadata}
                  closeDialog={()=>this.setState({showMoreLogs: false})}
                />
              }
              { this.state.showCopyCellToNotebook &&
                <CopyCellToNotebookDialog
                  cell={this.state.cell}
                  notebook_metadata={this.props.notebook_metadata}
                  closeDialog={()=>{
                      this.setState({showCopyCellToNotebook: false});
                      this.props.updateVersion();
                  }}
                  >

                </CopyCellToNotebookDialog>
              }
              {this.state.showNotebookUploadsDialog &&
               <NotebookUploads
                 notebook={this.props.notebook_metadata}
                 cell={this.state.cell}
                 closeDialog={() => this.setState({ showNotebookUploadsDialog: false })}
               />
              }

              <div className={classNames({selected: selected, "notebook-cell": true})} >
                <div className='notebook-input'>
                  { this.state.currently_editing && selected &&
                        <>
                          <Navbar className="toolbar">{ ace_toolbar }</Navbar>
                          <div className="notebook-editor" onPaste={this.pasteEvent}>
                            <form>
                              <VeloAce
                                toolbar={ace_toolbar}
                                mode={this.ace_type(this.state.cell.type)}
                                aceConfig={this.aceConfig}
                                text={this.state.input}
                                onChange={(value) => {this.setState({input: value});}}
                                commands={[{
                                    name: 'saveAndExit',
                                    bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
                                    exec: (editor) => {
                                        let cell = this.state.cell;
                                        cell.input = this.state.ace.getValue();
                                        this.saveCell(cell);
                                    },
                                }]}
                              />
                            </form>
                          </div>
                        </>
                  }
                  { selected && !this.state.currently_editing &&
                    <Navbar className="toolbar">{non_editing_toolbar}</Navbar>
                  }
                </div>

                <div ref={this.myRef}
                  className={classNames({
                    collapsed: this.state.collapsed,
                    "notebook-output": true,
                })}
                     type="button"
                     onKeyPress={()=>this.props.setSelectedCellId(
                         this.state.cell.cell_id)}
                     onClick={()=>this.props.setSelectedCellId(
                         this.state.cell.cell_id)}
                >
                  <NotebookReportRenderer
                    env={this.getEnv()}
                    refresh={()=>this.recalculate(this.state.cell)}
                    notebook_id={this.props.notebook_id}
                    completion_reporter={this.registerCompletions}
                    cell={this.state.cell}/>
                  { selected &&
                    <>
                      {_.map(this.state.cell.messages, (msg, idx) => {
                          // Extract the log level and render the
                          // classes based on it
                          let className = "";
                          let match = msg.match(/(ERROR|DEBUG|WARN|INFO|DEFAULT):(.+)/);
                          if (match) {
                              msg = match[2];

                              switch (match[1]) {
                              case "ERROR":
                                  className = "error-message"; break;
                              case "DEBUG":
                                  className = "debug-message"; break;
                              case "INFO":
                                  className = "info-message"; break;
                              case "WARN":
                                  className = "info-message"; break;
                              case "DEFAULT":
                                  className = "info-message"; break;
                              }
                          } else {
                            className = "info-message";
                          };
                          return <div key={idx} className={className}>
                                   {msg}
                                 </div>;
                      })}

                      <Button
                        variant="secondary"
                        onClick={()=>this.setState({showMoreLogs: true})}>
                        {T("Logs")}
                      </Button>
                    </>
                  }
                </div>
              </div>
            </div>
        );
    }
};
