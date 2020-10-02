import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import classNames from "classnames";
import VeloAce from '../core/ace.js';
import NotebookReportRenderer from './notebook-report-renderer.js';

import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Dropdown from 'react-bootstrap/Dropdown';
import FormControl from 'react-bootstrap/FormControl';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';

const cell_types = ["Markdown", "VQL", "Artifact"];


export default class NotebookCellRenderer extends React.Component {
    static propTypes = {
        cell_metadata: PropTypes.object,
        notebook_id: PropTypes.string,
        selected_cell_id: PropTypes.string,
        setSelectedCellId: PropTypes.func,

        upCell: PropTypes.func,
        downCell: PropTypes.func,
        deleteCell: PropTypes.func,
        addCell: PropTypes.func,
    };

    state = {
        cell_timestamp: 0,

        // The full cell data.
        cell: {},

        // The ace editor.
        ace: {},

        currently_editing: false,
        input: "",
    }

    componentDidMount() {
        this.fetchCellContents();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        // Do not update the editor if we are currently editing it -
        // otherwise it will wipe the text.
        if (this.state.currently_editing) {
            return;
        }

        let this_timestamp = this.props.cell_metadata && this.props.cell_metadata.timestamp;
        if (this_timestamp !== this.state.cell_timestamp) {
            this.setState({cell_timestamp: this_timestamp});
            this.fetchCellContents();
        }
    };

    fetchCellContents = () => {
        api.get("api/v1/GetNotebookCell", {
            notebook_id: this.props.notebook_id,
            cell_id: this.props.cell_metadata.cell_id,
        }).then((response) => {
            let cell = response.data;
            this.setState({cell: cell, input: cell.input});
        });
    };

    setEditing = (edit) => {
        this.setState({currently_editing: edit});
    };

    aceConfig = (ace) => {
        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25
        });

        this.setState({ace: ace});
    };

    ace_type = (type) => {
        if (type === "VQL") {
            return "sql";
        }
        if (type === "Markdown") {
            return "markdown";
        }
        if (type === "Artifact") {
            return "yaml";
        }

        return "yaml";
    }

    saveCell = () => {
        let cell = this.state.cell;
        cell.input = this.state.ace.getValue();
        cell.output = "Loading";
        cell.timestamp = 0;
        this.setState({cell: cell});

        api.post('/api/v1/UpdateNotebookCell', {
            notebook_id: this.props.notebook_id,
            cell_id: this.state.cell.cell_id,
            type: this.state.cell.type || "Markdown",
            currently_editing: false,
            input: this.state.cell.input,
        }).then( (response) => {
            this.setState({cell: response.data, currently_editing: false});
        });
    };

    deleteCell = () => {
        this.props.deleteCell(this.state.cell.cell_id);
        this.setState({currently_editing: false});
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
                        'api/v1/UploadNotebookAttachment', request
                    ).then((response) => {
                        this.state.ace.insert("\n!["+blob.name+"]("+response.data.url+")\n");
                    }, function failure(response) {
                        console.log("Error " + response.data);
                    });
                };
                reader.readAsDataURL(blob);
            }
        }
    };

    render() {
        let selected = this.state.cell.cell_id === this.props.selected_cell_id;

        // There are 3 states for the cell:
        // 1. The cell is selected but not being edited: Show the cell manipulation toolbar.
        // 2. The cell is being edited: Show the editing toolbar
        // 3. The cell is not selected or edited - no decorations.

        let toolbar = <></>;

        if (selected && !this.state.currently_editing) {
            toolbar = (
                <ButtonGroup>
                  <Button title="Cancel"
                          onClick={() => {this.props.setSelectedCellId("");}}
                          variant="default">
                    <FontAwesomeIcon icon="window-close"/>
                  </Button>
                  <Button title="Stop Calculating"
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

                  <Dropdown title="Add Cell" variant="default">
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="plus"/>
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      <Dropdown.Item
                        title="Markdown"
                        onClick={() => {
                            this.props.addCell(this.state.cell.cell_id, "Markdown");
                        }}>
                        Markdown
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="VQL"
                        onClick={() => {
                            this.props.addCell(this.state.cell.cell_id, "VQL");
                        }}>
                        VQL
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="Artifact"
                        onClick={() => {
                            this.props.addCell(this.state.cell.cell_id, "Artifact");
                        }}>
                        Artifact
                      </Dropdown.Item>
                      <hr/>
                      <Dropdown.Item
                        title="Add Cell From Hunt"
                        onClick={this.addCellFromHunt}>
                        Add Cell From Hunt
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="Add Cell From Flow"
                        onClick={this.addCellFromFlow}>
                        Add Cell From Flow
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="Add Cell From Artifact"
                        onClick={this.addCellFromArtifact}>
                        Add Cell From Artifact
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>
            );
        }


        let ace_toolbar = (
            <>
              <ButtonGroup>
                <Button title="Undo"
                        onClick={() => {this.setEditing(false); }}
                        variant="default">
                  <FontAwesomeIcon icon="window-close"/>
                </Button>

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
                      return <option value={v} key={idx}>
                               {v}
                             </option>;
                  })}
                </FormControl>
              </ButtonGroup>
            </>
        );

        return (
            <>
              <div className={classNames({selected: selected, "notebook-cell": true})} >
                <div className='notebook-input'>
                  {toolbar}
                  { this.state.currently_editing && selected &&
                    <div className="notebook-editor" onPaste={this.pasteEvent}>
                      <form>
                        <VeloAce
                          toolbar={ace_toolbar}
                          mode={this.ace_type(this.state.cell.type)}
                          aceConfig={this.aceConfig}
                          text={this.state.input}
                          onChange={(value) => {this.setState({input: value});}}
                        />
                      </form>
                    </div>
                  }
                </div>

                <div className={classNames({collapsed: this.state.collapsed,
                                            "notebook-output": true})}
                     onClick={() => {this.props.setSelectedCellId(this.state.cell.cell_id);}}
                >
                  <NotebookReportRenderer
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
