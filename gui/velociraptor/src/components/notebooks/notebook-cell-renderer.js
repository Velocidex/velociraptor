import './notebook-cell-renderer.css';

import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import classNames from "classnames";
import VeloAce from '../core/ace.js';
import NotebookReportRenderer from './notebook-report-renderer.js';
import { SettingsButton } from '../core/ace.js';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Dropdown from 'react-bootstrap/Dropdown';
import FormControl from 'react-bootstrap/FormControl';
import Navbar from 'react-bootstrap/Navbar';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';
import CreateArtifactFromCell from './create-artifact-from-cell.js';
import AddCellFromFlowDialog from './add-cell-from-flow.js';
import Completer from '../artifacts/syntax.js';
import { getHuntColumns } from '../hunts/hunt-list.js';
import VeloTimestamp from "../utils/time.js";
import { AddTimelineDialog, AddVQLCellToTimeline } from "./timelines.js";

import axios from 'axios';
import api from '../core/api-service.js';

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
        this.source = axios.CancelToken.source();
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
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: (row) => {
                this.addCellFromHunt(row);
            },
        };

        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>Add cell from Hunt</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <div className="no-margins selectable">
                  <BootstrapTable
                    hover
                    condensed
                    keyField="hunt_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={this.state.hunts}
                    columns={getHuntColumns()}
                    filter={ filterFactory() }
                    selectRow={ selectRow }
                  />
                  { _.isEmpty(this.state.hunts) &&
                    <div className="no-content">No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.</div>}
                </div>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.addCellFromHunt}>
                  Submit
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class NotebookCellRenderer extends React.Component {
    static propTypes = {
        cell_metadata: PropTypes.object,
        notebook_id: PropTypes.string,
        notebook_metadata: PropTypes.object.isRequired,
        selected_cell_id: PropTypes.string,
        setSelectedCellId: PropTypes.func,

        upCell: PropTypes.func,
        downCell: PropTypes.func,
        deleteCell: PropTypes.func,

        // func(notebook_cell_id, text, type, env)
        addCell: PropTypes.func,
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
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
        this.fetchCellContents();
    }

    componentWillUnmount() {
        this.source.cancel();
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

        // We detect the cell has changed by looking at both cell id and the cell timestamp.
        let props_cell_timestamp = this.props.cell_metadata && this.props.cell_metadata.timestamp;
        let props_cell_id = this.props.cell_metadata && this.props.cell_metadata.cell_id;

        if (props_cell_timestamp !== this.state.cell_timestamp ||
            props_cell_id !== current_cell_id) {

            // Prevent further updates to this cell by setting the
            // cell id and timestamp.
            this.setState({cell_timestamp: props_cell_timestamp,
                           loading: true,
                           cell_id: props_cell_id});
            this.fetchCellContents();
        }
    };

    fetchCellContents = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetNotebookCell", {
            notebook_id: this.props.notebook_id,
            cell_id: this.props.cell_metadata.cell_id,
        }, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let cell = response.data;
            this.setState({cell: cell, input: cell.input, loading: false});
        });
    };

    setEditing = (edit) => {
        this.setState({currently_editing: edit});
    };

    getPlaceholder = () => {
        let type = this.ace_type(this.state.cell && this.state.cell.type);
        if (type === "vql") {
            return "Type VQL to evaluate on the server (press ? for help)";
        };
        if (type === "markdown") {
            return "Enter markdown text to render in the notebook";
        };
        return type;
    }

    aceConfig = (ace) => {
        // Attach a completer to ACE.
        let completer = new Completer();
        completer.initializeAceEditor(ace, {});

        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25,
            placeholder: this.getPlaceholder(),
        });

        this.setState({ace: ace});
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

    recalculate = () => {
        let cell = this.state.cell;
        cell.output = "Loading";
        cell.timestamp = 0;
        this.setState({cell: cell});

        api.post('v1/UpdateNotebookCell', {
            notebook_id: this.props.notebook_id,
            cell_id: this.state.cell.cell_id,
            type: this.state.cell.type || "Markdown",
            env: this.state.cell.env,
            currently_editing: false,
            input: this.state.cell.input,
        }, this.source.token).then( (response) => {
            if (response.cancel) {
                return;
            }

            this.setState({cell: response.data,
                           currently_editing: false});
        });

    }

    saveCell = () => {
        let cell = this.state.cell;
        cell.input = this.state.ace.getValue();
        cell.output = "Loading";
        cell.timestamp = 0;
        this.setState({cell: cell});

        api.post('v1/UpdateNotebookCell', {
            notebook_id: this.props.notebook_id,
            cell_id: cell.cell_id,
            type: cell.type || "Markdown",
            env: this.state.cell.env,
            currently_editing: false,
            input: cell.input,
        }, this.source.token).then( (response) => {
            if (response.cancel) {
                return;
            }

            this.setState({cell: response.data, currently_editing: false});
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
        }, this.source.token).then(response=>{
            if (response.cancel) {
                return;
            }
            this.fetchCellContents();
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
                reader.onload = (event) => {
                    let request = {
                        data: reader.result.split(",")[1],
                        notebook_id: this.props.notebook_id,
                        filename: blob.name,
                        size: blob.size,
                    };

                    api.post(
                        'v1/UploadNotebookAttachment', request, this.source.token
                    ).then((response) => {
                        if (response.cancel) return;
                        this.state.ace.insert("\n!["+blob.name+"]("+response.data.url+")\n");
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


    render() {
        let selected = this.state.cell.cell_id === this.props.selected_cell_id;

        // There are 3 states for the cell:
        // 1. The cell is selected but not being edited: Show the cell manipulation toolbar.
        // 2. The cell is being edited: Show the editing toolbar
        // 3. The cell is not selected or edited - no decorations.
        let non_editing_toolbar = (
            <>
            <ButtonGroup>
              <Button title="Cancel"
                      onClick={() => {this.props.setSelectedCellId("");}}
                      variant="default">
                <FontAwesomeIcon icon="window-close"/>
              </Button>

              <Button title="Recalculate"
                      disabled={this.state.cell.calculating}
                      onClick={this.recalculate}
                      variant="default">
                <FontAwesomeIcon icon="sync"/>
              </Button>

              <Button title="Stop Calculating"
                      disabled={!this.state.cell.calculating}
                      onClick={this.stopCalculating}
                      variant="default">
                <FontAwesomeIcon icon="stop"/>
              </Button>
              { !this.state.collapsed &&
                <Button title="Collapse"
                        onClick={() => this.setState({collapsed: true})}
                        variant="default">
                  <FontAwesomeIcon icon="compress"/>
                </Button>
              }
              { this.state.collapsed &&
                <Button title="Expand"
                        onClick={() => this.setState({collapsed: false})}
                        variant="default">
                  <FontAwesomeIcon icon="expand"/>
                </Button>
              }
              <Button title="Edit Cell"
                      disabled={this.state.cell.calculating}
                      onClick={() => { this.setEditing(true); }}
                      variant="default">
                <FontAwesomeIcon icon="pencil-alt"/>
              </Button>

              <Button title="Up Cell"
                      onClick={() => {
                          this.props.upCell(this.state.cell.cell_id);
                      }}
                      variant="default">
                <FontAwesomeIcon icon="arrow-up"/>
              </Button>

              <Button title="Down Cell"
                      onClick={() => {
                          this.props.downCell(this.state.cell.cell_id);
                      }}
                      variant="default">
                <FontAwesomeIcon icon="arrow-down"/>
              </Button>

              {this.state.cell && this.state.cell.type === "vql" &&
               <Button title="Add Timeline"
                       onClick={()=>this.setState({showAddCellToTimeline: true})}
                       variant="default">
                 <FontAwesomeIcon icon="calendar-alt"/>
               </Button>}

              <Dropdown title="Add Cell" variant="default">
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
                  <Dropdown.Item
                    title="Add Timeline"
                    onClick={()=>this.setState({showAddTimeline: true})}>
                    Add Timeline
                  </Dropdown.Item>
                  <Dropdown.Item
                    title="Add Cell From This Cell"
                    onClick={this.addCellFromCell}>
                    Add Cell From This Cell
                  </Dropdown.Item>
                  <Dropdown.Item
                    title="Add Cell From Hunt"
                    onClick={()=>this.setState({showAddCellFromHunt: true})}>
                    Add Cell From Hunt
                  </Dropdown.Item>
                  <Dropdown.Item
                    title="Add Cell From Flow"
                    onClick={()=>this.setState({showAddCellFromFlow: true})}>
                    Add Cell From Flow
                  </Dropdown.Item>
                  {this.state.cell && this.state.cell.type === "VQL" &&
                   <Dropdown.Item
                     title="Create Artifact from VQL"
                     onClick={()=>this.setState({showCreateArtifactFromCell: true})}>
                     Create Artifact from VQL
                   </Dropdown.Item>}
                </Dropdown.Menu>
              </Dropdown>
            </ButtonGroup>
            <ButtonGroup className="float-right">
              <Button title="Rendered" disabled
                      variant="outline-info">
                <VeloTimestamp usec={this.state.cell.timestamp * 1000} />
                { this.state.cell.duration &&
                  <span>&nbsp;({this.state.cell.duration}s) </span> }
              </Button>
            </ButtonGroup>
            </>
        );

        let ace_toolbar = (
            <>
              <ButtonGroup>
                <Button title="Undo"
                        onClick={() => {this.setEditing(false); }}
                        variant="default">
                  <FontAwesomeIcon icon="window-close"/>
                </Button>

                <SettingsButton ace={this.state.ace}/>

                <Button title="Stop Calculating"
                        onClick={this.stopCalculating}
                        variant="default">
                  <FontAwesomeIcon icon="stop"/>
                </Button>

                <Button title="Save"
                        onClick={this.saveCell}
                        variant="default">
                  <FontAwesomeIcon icon="save"/>
                </Button>
              </ButtonGroup>

              <ButtonGroup className="float-right">
                <Button title="Delete Cell"
                        onClick={this.deleteCell}
                        variant="default">
                  <FontAwesomeIcon icon="trash"/>
                </Button>

                <FormControl as="select"
                             ref={ (el) => this.element=el }
                             value={this.state.cell.type}
                             onChange={() => {
                                 let cell = this.state.cell;
                                 cell.type = this.element.value;
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
            <>{ this.state.showAddCellFromHunt &&
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
                                        this.saveCell();
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

                <div className={classNames({collapsed: this.state.collapsed,
                                            "notebook-output": true})}
                     onClick={() => {this.props.setSelectedCellId(this.state.cell.cell_id);}}
                >
                  <NotebookReportRenderer
                    refresh={this.recalculate}
                    notebook_id={this.props.notebook_id}
                    cell={this.state.cell}/>
                  { selected &&
                    _.map(this.state.cell.messages, (msg, idx) => {
                        return <div key={idx} className="error-message">
                        {msg}
                        </div>;
                    })}
                </div>
              </div>
            </>
        );
    }
};
