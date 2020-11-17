import "./clients-list.css";

import online from './img/online.png';
import any from './img/any.png';

import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import VeloClientStatusIcon from "./client-status.js";
import { formatColumns } from "../core/table.js";
import filterFactory from 'react-bootstrap-table2-filter';
import BootstrapTable from 'react-bootstrap-table-next';
import ClientLink from './client-link.js';
import Spinner from '../utils/spinner.js';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import LabelForm from '../utils/labels.js';
import Row from 'react-bootstrap/Row';
import Pagination from 'react-bootstrap/Pagination';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Alert from 'react-bootstrap/Alert';

import { sizePerPageRenderer } from '../core/table.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


const UNSORTED = 0;
const SORT_UP = 1;
const SORT_DOWN = 2;

const UNFILTERED = 0;
const ONLINE = 1;

class LabelClients extends Component {
    static propTypes = {
        affectedClients: PropTypes.array,
        onResolve: PropTypes.func.isRequired,
    }

    labelClients = () => {
        let labels = this.state.labels;
        if (this.state.new_label) {
            labels.push(this.state.new_label);
        }

        let client_ids = _.map(this.props.affectedClients,
                               client => client.client_id);

        api.post("v1/LabelClients", {
            client_ids: client_ids,
            operation: "set",
            labels: labels,
        }).then((response) => {
            this.props.onResolve();
        });
    }

    state = {
        labels: [],
        new_label: "",
    }


    render() {
        let clients = this.props.affectedClients || [];
        let columns = formatColumns([
            {dataField: "last_seen_at", text: "Online", sort: true,
             formatter: (cell, row) => {
                 return <VeloClientStatusIcon client={row}/>;
             }},
            {dataField: "client_id", text: "Client ID"},
            {dataField: "os_info.fqdn", text: "Hostname", sort: true},
        ]);

        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>Label Clients</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Existings</Form.Label>
                  <Col sm="8">
                    <LabelForm
                      value={this.state.labels}
                      onChange={(value) => this.setState({labels: value})}/>
                  </Col>
                </Form.Group>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">A new label</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                      rows={1}
                      value={this.state.new_label}
                      onChange={(e) => this.setState({new_label: e.currentTarget.value})}
                    />
                  </Col>
                </Form.Group>
                <BootstrapTable
                    hover
                    condensed
                    keyField="client_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={clients}
                    columns={columns}
                />
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  Close
                </Button>
                <Button variant="primary"
                        onClick={this.labelClients}>
                  Run it!
                </Button>
              </Modal.Footer>
            </Modal>

        );
    }
}


const POLL_TIME = 2000;

class DeleteClients extends Component {
    static propTypes = {
        affectedClients: PropTypes.array,
        onResolve: PropTypes.func.isRequired,
    }

    state = {
        flow_id: null,
    }

    deleteClients = () => {
        let client_ids = _.map(this.props.affectedClients,
                               client => client.client_id);

        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.DeleteClient"],
            parameters: {"env": [
                { "key": "ClientIdList", "value": client_ids.join(",")},
                { "key": "ReallyDoIt", "value": "Y"},
            ]},
        }).then((response) => {
            // Hold onto the flow id.
            this.setState({flow_id: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.flow_id,
                }).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        this.setState({flow_context: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id, we can stop polling.
                    clearInterval(this.recursive_download_interval);
                    this.recursive_download_interval = undefined;

                    this.props.onResolve();
                });
            }, POLL_TIME);
        });
    }

    render() {
        let clients = this.props.affectedClients || [];
        let columns = formatColumns([
            {dataField: "last_seen_at", text: "Online", sort: true,
             formatter: (cell, row) => {
                 return <VeloClientStatusIcon client={row}/>;
             }},
            {dataField: "client_id", text: "Client ID"},
            {dataField: "os_info.fqdn", text: "Hostname", sort: true},
        ]);

        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>Delete Clients</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Alert variant="danger">
                  You are about to permanentsly delete the following clients
                </Alert>
                <div className="deleted-client-list">
                <BootstrapTable
                    hover
                    condensed
                    keyField="client_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={clients}
                    columns={columns}
                />
                  </div>
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  Close
                </Button>
                <Button variant="primary"
                        disabled={this.state.flow_id}
                        onClick={this.deleteClients}>
                  {this.state.flow_id && <FontAwesomeIcon icon="spinner" spin/>}
                  Yeah do it!
                </Button>
              </Modal.Footer>
            </Modal>

        );
    }
}


const pageListRenderer = ({
    pages,
    currentPage,
    totalRows,
    pageSize,
    onPageChange
}) => {
    // just exclude <, <<, >>, >
    const pageWithoutIndication = pages.filter(p => typeof p.page !== 'string');
    let totalPages = parseInt(totalRows / pageSize);

    // Only allow changing to a page if there are any rows in that
    // page.
    if (totalPages * pageSize + 1 > totalRows) {
        totalPages--;
    }

    return (
        <Pagination>
          <Pagination.First onClick={()=>onPageChange(0)}/>
          {
              pageWithoutIndication.map((p, idx)=>(
                <Pagination.Item
                  key={idx}
                  active={p.active}
                  onClick={ () => onPageChange(p.page) } >
                  { p.page }
                </Pagination.Item>
            ))
          }
          <Pagination.Next onClick={()=>onPageChange(pageWithoutIndication.length)}/>
          <Form.Control
            as="input"
            className="pagination-form"
            placeholder="Goto Page"
            value={currentPage || ""}
            onChange={e=> {
                let page = parseInt(e.currentTarget.value || 0);
                if (page >= 0) {
                    onPageChange(page);
                }
            }}/>

        </Pagination>
    );
};


class VeloClientList extends Component {
    static propTypes = {
        query: PropTypes.string.isRequired,
        version: PropTypes.any,
        setClient: PropTypes.func.isRequired,
        setSearch: PropTypes.func.isRequired,
    }

    state = {
        clients: [],
        selected: [],
        loading: false,
        showLabelDialog: false,
        showDeleteDialog: false,
        focusedClient: null,

        start_row: 0,
        page_size: 10,
        sort: UNSORTED,
        filter: UNFILTERED,
    };

    componentDidMount = () => {
        let query = this.props.match && this.props.match.params &&
            this.props.match.params.query;
        if (query && query !== this.state.query) {
            this.props.setSearch(query);
        };
        this.searchClients();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.query !== prevProps.query ||
            this.props.version !== prevProps.version ||
            this.state.page_size !== prevState.page_size ||
            this.state.sort !== prevState.sort ||
            this.state.filter !== prevState.filter ||
            this.state.start_row !== prevState.start_row) {
            this.searchClients();
        }
    }

    searchClients = () => {
        var query = this.props.query;
        if (!query) {
            return;
        }

        this.setState({loading: true});
        api.get('/v1/SearchClients', {
            query: query,
            limit: this.state.page_size,
            offset: this.state.start_row,
            sort: this.state.sort,
            filter: this.state.filter,
        }).then(resp => {
            let items = resp.data && resp.data.items;
            items = items || [];
            this.setState({loading: false, clients: items});
        });
    }

    openClientInfo = (client) => {
        this.props.setClient(client);

        // Navigate to the host_info page.
        this.props.history.push('/host/' + client.client_id);
    }

    handleOnSelect = (row, isSelect) => {
        if (isSelect) {
            this.setState(() => ({
                selected: [...this.state.selected, row.client_id]
            }));
        } else {
            this.setState(() => ({
                selected: this.state.selected.filter(x => x !== row.client_id)
            }));
        }
    }

    handleOnSelectAll = (isSelect, rows) => {
        const ids = rows.map(r => r.client_id);
        if (isSelect) {
            this.setState(() => ({
                selected: ids
            }));
        } else {
            this.setState(() => ({
                selected: []
            }));
        }
    }

    getAffectedClients = () => {
        let result = [];

        _.each(this.state.selected, (client_id) => {
            _.each(this.state.clients, (client) => {
                if(client.client_id === client_id) {
                    result.push(client);
                };
            });
        });

        return result;
    }

    // Do not refresh the entire client table because it will reorder
    // the clients, just carefully remove the label from the existing
    // records for smoother ui.
    removeLabel = (label, client) => {
        api.post("v1/LabelClients", {
            client_ids: [client.client_id],
            operation: "remove",
            labels: [label],
        }).then((response) => {
            client.labels = client.labels.filter(x=>x !== label);
            this.setState({showLabelDialog: false});
        });
    }

    render() {
        let columns = getClientColumns();
        let total_size = this.state.clients.length + this.state.page_size + this.state.start_row;

        columns[0].headerFormatter = (column, colIndex, { sortElement, filterElement }) => {
            return (
                <Button variant="outline-default"
                        onClick={()=>{
                            let filter = this.state.filter === ONLINE ? UNFILTERED : ONLINE;
                            this.setState({filter: filter});
                        }}
                  >
                  <span className="button-label">
                    { this.state.filter === ONLINE ?
                      <img className="icon-small" src={online} alt="online" />  :
                      <img className="icon-small" src={any} alt="any state" />
                    }
                  </span>
                </Button>
            );
        };

        columns[2].onSort = (field, order) => {
            this.props.setSearch("host:*");
            if (order === "asc") {
                this.setState({sort: SORT_UP});
            } else if (order === "desc") {
                this.setState({sort: SORT_DOWN});
            };
        };

        columns.push({
            dataField: "labels", text: "Labels",
            sort:true, filtered: true,
            formatter: (cell, row) => {
                return _.map(cell, (label, idx) => {
                    return <Button size="sm" key={idx}
                                   onClick={() => this.removeLabel(label, row)}
                                   variant="default">
                             <span className="button-label">{label}</span>
                             <span className="button-label">
                               <FontAwesomeIcon icon="window-close"/>
                             </span>
                           </Button>;
                });
            }});

        const selectRow = {
            mode: "checkbox",
            clickToSelect: true,
            classes: "row-selected",
            selected: this.state.selected,
            onSelect: this.handleOnSelect,
            onSelectAll: this.handleOnSelectAll
        };

        let affected_clients = this.getAffectedClients();

        return (
            <>
              { this.state.showDeleteDialog &&
                <DeleteClients
                  affectedClients={affected_clients}
                  onResolve={() => {
                      this.setState({showDeleteDialog: false});
                      this.searchClients();
                  }}/>}
              { this.state.showLabelDialog &&
                <LabelClients
                  affectedClients={affected_clients}
                  onResolve={() => {
                      this.setState({showLabelDialog: false});
                      this.searchClients();
                  }}/>}
              <Spinner loading={this.state.loading}/>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="Label Clients"
                          disabled={_.isEmpty(this.state.selected)}
                          onClick={() => this.setState({showLabelDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="tags"/>
                  </Button>
                  <Button title="Delete Clients"
                          disabled={_.isEmpty(this.state.selected)}
                          onClick={() => this.setState({showDeleteDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>

                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin selectable">
                <BootstrapTable
                  hover
                  remote
                  condensed
                  noDataIndication="Table is Empty"
                  keyField="client_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.state.clients}
                  columns={columns}
                  selectRow={ selectRow }
                  filter={ filterFactory() }
                  onTableChange={(type, { page, sizePerPage }) => {
                      this.setState({start_row: page * sizePerPage});
                  }}

                  pagination={ paginationFactory({
                      sizePerPage: this.state.page_size,
                      totalSize: total_size,
                      currSizePerPage: this.state.page_size,
                      onSizePerPageChange: value=>{
                          this.setState({page_size: value});
                      },
                      pageStartIndex: 0,
                      pageListRenderer: ({pages, onPageChange})=>pageListRenderer({
                          pageSize: this.state.page_size,
                          pages: pages,
                          currentPage: this.state.start_row / this.state.page_size,
                          onPageChange: onPageChange}),
                      sizePerPageRenderer
                  }) }

                />
              </div>
            </>
        );
    }
};


export default withRouter(VeloClientList);

export function getClientColumns() {
    return formatColumns([
        {dataField: "last_seen_at", text: "Online",
         formatter: (cell, row) => {
             return <VeloClientStatusIcon client={row}/>;
         }},
        {dataField: "client_id", text: "Client ID",
         formatter: (cell, row) => {
             return <ClientLink client_id={cell}/>;
         }},
        {dataField: "os_info.fqdn", text: "Hostname",
         sort: true},
        {dataField: "os_info.release", text: "OS Version"},
    ]);
}
