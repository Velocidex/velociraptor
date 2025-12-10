import "./clients-list.css";

import {CancelToken} from 'axios';
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";
import T from '../i8n/i8n.jsx';
import api from '../core/api-service.jsx';
import VeloClientStatusIcon from "./client-status.jsx";
import { TablePaginationControl } from '../core/paged-table.jsx';

import ClientLink from '../clients/client-link.jsx';
import Spinner from '../utils/spinner.jsx';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import LabelForm from '../utils/labels.jsx';
import Row from 'react-bootstrap/Row';
import Alert from 'react-bootstrap/Alert';
import UserConfig from '../core/user.jsx';
import Table from 'react-bootstrap/Table';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


const UNSORTED = 0;
const UNFILTERED = 0;
const ONLINE = 1;


class SimpleClientList extends Component {
    static propTypes = {
        clients: PropTypes.array,
    }

    render() {
        return (
            <Table className="paged-table">
              <thead>
                <tr className="paged-table-header">
                  <th>{T("Online")}</th>
                  <th>{T("Client ID")}</th>
                  <th>{T("Hostname")}</th>
                </tr>
              </thead>
              <tbody className="fixed-table-body">
                {_.map(this.props.clients, (c, i)=>{
                    return (
                        <tr key={i}>
                          <td>
                            <Button variant="outline-default"
                                    className="online-status">
                              <span className="button-label">
                                <VeloClientStatusIcon client={c}/>
                              </span>
                            </Button>
                          </td>
                          <td><ClientLink client_id={c && c.client_id}/></td>
                          <td>{c && c.os_info && c.os_info.hostname}</td>
                        </tr>);
                })}
              </tbody>
            </Table>
        );
    }
}

export class LabelClients extends Component {
    static propTypes = {
        affectedClients: PropTypes.array,
        onResolve: PropTypes.func.isRequired,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    labelClients = () => {
        let labels = [...this.state.labels];
        if (this.state.new_label) {
            labels.push(this.state.new_label);
        }

        let client_ids = _.map(this.props.affectedClients,
                               client => client.client_id);

        api.post("v1/LabelClients", {
            client_ids: client_ids,
            operation: "set",
            labels: labels,
        }, this.source.token).then((response) => {
            this.props.onResolve();
        });
    }

    state = {
        labels: [],
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Label Clients")}</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Select Label")}</Form.Label>
                  <Col sm="8">
                    <LabelForm
                      value={this.state.labels}
                      onChange={(value) => this.setState({labels: value})}/>
                  </Col>
                </Form.Group>
                <SimpleClientList clients={this.props.affectedClients || []}/>
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.labelClients}>
                  {T("Add it!")}
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

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    deleteClients = () => {
        let client_ids = _.map(this.props.affectedClients,
                               client => client.client_id);

        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.DeleteClient"],
            specs: [{artifact: "Server.Utils.DeleteClient",
                     parameters: {"env": [
                         { "key": "ClientIdList", "value": client_ids.join(",")},
                         { "key": "ReallyDoIt", "value": "Y"},
                     ]}}],
        }, this.source.token).then((response) => {
            // Hold onto the flow id.
            this.setState({flow_id: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.flow_id,
                }, this.source.token).then((response) => {
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
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Delete Clients")}</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Alert variant="danger">
                  {T("DeleteMessage")}
                </Alert>
                <div className="deleted-client-list">
                  <SimpleClientList clients={clients}/>
                </div>
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={this.state.flow_id}
                        onClick={this.deleteClients}>
                  {this.state.flow_id && <FontAwesomeIcon icon="spinner" spin/>}
                  {T("Yeah do it!")}
                </Button>
              </Modal.Footer>
            </Modal>

        );
    }
}

class KillClients extends Component {
    static propTypes = {
        affectedClients: PropTypes.array,
        onResolve: PropTypes.func.isRequired,
    }

    state = {
        flow_id: null,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    killClients = () => {
        let client_ids = _.map(this.props.affectedClients,
                               client => client.client_id);

        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["Server.Utils.KillClient"],
            specs: [{artifact: "Server.Utils.KillClient",
                     parameters: {"env": [
                         { "key": "ClientIdList", "value": client_ids.join(",")},
                     ]}}],
        }, this.source.token).then((response) => {
            // Hold onto the flow id.
            this.setState({flow_id: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: "server",
                    flow_id: this.state.flow_id,
                }, this.source.token).then((response) => {
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
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Kill Clients")}</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Alert variant="danger">
                  {T("KillMessage")}
                </Alert>
                <div className="deleted-client-list">
                  <SimpleClientList clients={clients}/>
                </div>
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={this.state.flow_id}
                        onClick={this.killClients}>
                  {this.state.flow_id && <FontAwesomeIcon icon="spinner" spin/>}
                  {T("Yeah do it!")}
                </Button>
              </Modal.Footer>
            </Modal>

        );
    }
}

class VeloClientList extends Component {
    static contextType = UserConfig;

    static propTypes = {
        query: PropTypes.string.isRequired,
        version: PropTypes.any,
        setClient: PropTypes.func.isRequired,
        setSearch: PropTypes.func.isRequired,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    state = {
        clients: [],
        selected: [],
        selected_icon: "square",
        loading: false,
        showLabelDialog: false,
        showDeleteDialog: false,
        focusedClient: null,

        start_row: 0,
        page_size: 10,
        sort: UNSORTED,
        filter: UNFILTERED,

        selected_idx: -1,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        let query = this.props.match && this.props.match.params &&
            this.props.match.params.query;
        if (query && query !== this.state.query) {
            this.props.setSearch(query);
        };
        this.searchClients();
    }

    componentWillUnmount() {
        this.source.cancel();
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

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        this.setState({loading: true});
        api.get('/v1/SearchClients', {
            query: query,
            limit: this.state.page_size,
            offset: this.state.start_row,
            sort: this.state.sort,
            filter: this.state.filter,

            // Return all the matching client records.
            name_only: false,
        }, this.source.token).then(resp => {
            if (resp.cancel) return;
            let res = resp.data || {};
            this.setState({
                loading: false,
                search_term: res.search_term,
                total: res.total,
                clients: res.items || []});
        });
    }

    renderSearchStats = ()=>{
        let search_term = <></>;
        if (!_.isUndefined(this.state.search_term)) {
            search_term = <Button disabled={true} variant="outline-info">
                            {this.state.search_term.query
            } { this.state.search_term.filter == "ONLINE" &&
                <span>{T("ONLINE")}</span>}
            </Button>;
        }
        return <ButtonGroup>
                 {search_term}
               </ButtonGroup>;
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
        }, this.source.token).then((response) => {
            client.labels = client.labels.filter(x=>x !== label);
            this.setState({showLabelDialog: false});
        });
    }

    searchLabel = (label, client) => {
        this.props.setSearch("label:" + label);
    }

    isSelected = c=>{
        return _.includes(this.state.selected, c.client_id);
    }

    renderToolbar = ()=>{
        let affected_clients = this.getAffectedClients();
        return <>
                 { this.state.showDeleteDialog &&
                   <DeleteClients
                     affectedClients={affected_clients}
                     onResolve={() => {
                         this.setState({showDeleteDialog: false});
                         this.searchClients();
                     }}/>}
                 { this.state.showKillDialog &&
                   <KillClients
                     affectedClients={affected_clients}
                     onResolve={() => {
                         this.setState({showKillDialog: false});
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
                     <Button title={T("Label Clients")}
                             disabled={_.isEmpty(this.state.selected)}
                             onClick={() => this.setState({showLabelDialog: true})}
                             variant="default">
                       <FontAwesomeIcon icon="tags"/>
                     </Button>
                     <Button title={T("Delete Clients")}
                             disabled={_.isEmpty(this.state.selected)}
                             onClick={() => this.setState({showDeleteDialog: true})}
                             variant="default">
                       <FontAwesomeIcon icon="trash"/>
                     </Button>
                     { // Kiling clients requires the machine_state
                         // permission. Hide this button for users who do
                         // not have it.
                         this.context && this.context.traits &&
                             this.context.traits.Permissions &&
                             this.context.traits.Permissions.machine_state &&
                             <Button title={T("Kill Clients")}
                                     disabled={_.isEmpty(this.state.selected)}
                                     onClick={() => this.setState({showKillDialog: true})}
                                     variant="default">
                               <FontAwesomeIcon icon="ban"/>
                             </Button>
                     }
                     <TablePaginationControl
                       total_size={parseInt(this.state.total) || 0}
                       start_row={this.state.start_row}
                       page_size={this.state.page_size}
                       current_page={this.state.start_row / this.state.page_size}
                       onRowChange={row=>this.setState({start_row: row})}
                       onPageSizeChange={size=>this.setState({page_size: size})}
                     />
                   </ButtonGroup>
                   <ButtonGroup className="float-right">
                     {this.renderSearchStats()}
                   </ButtonGroup>
                 </Navbar>
               </>;
    }

    renderBasicTable = ()=>{
        return (
            <>
              <Table className="paged-table">
                <thead>
                  <tr className="paged-table-header">
                    <th>
                      <Button variant="outline-default"
                              className="checkbox"
                              onClick={()=>{
                                  // Now clients are selected - select all
                                  if(this.state.selected.length === 0) {
                                      this.setState({
                                          selected: _.map(this.state.clients, c=>c.client_id),
                                          selected_icon: "square-check"});
                                  } else {
                                      // Reset all clients
                                      this.setState({
                                          selected: [],
                                          selected_icon: "square",
                                      });
                                  }}}
                      >
                        <span className="button-label">
                          <FontAwesomeIcon icon={["far", this.state.selected_icon]}/>
                        </span>
                      </Button>
                    </th>
                    <th>
                      <Button variant="outline-default"
                              onClick={()=>{
                                  let filter = this.state.filter === ONLINE ? UNFILTERED : ONLINE;
                                  this.setState({filter: filter});
                              }}
                      >
                        <span className="button-label">
                          { this.state.filter === ONLINE ?
                            <span className="online-btn" alt="online" />  :
                            <span className="any-btn" alt="any state" />
                          }
                        </span>
                      </Button>
                    </th>
                    <th>{T("Client ID")}</th>
                    <th>{T("Hostname")}</th>
                    <th>{T("FQDN")}</th>
                    <th>{T("OS Version")}</th>
                    <th>{T("Labels")}</th>
                  </tr>
                </thead>
                <tbody className="fixed-table-body">
                  {_.map(this.state.clients, (c, i)=>{
                      let client_id = c.client_id;
                      let is_selected = _.includes(this.state.selected, client_id);
                      let num_clients = this.state.clients.length;

                      return (
                          <tr key={i}
                              className={(is_selected && "row-selected") || ""}
                              onClick={e=>{
                                  e.stopPropagation();
                                  // User pressed shift - we select
                                  // all clients from the previous
                                  // selection.
                                  if(e.shiftKey) {
                                      document.getSelection().removeAllRanges();

                                      let first = this.state.selected_idx;
                                      let last = i;
                                      if (first > last) {
                                          let tmp = first;
                                          first = last;
                                          last = tmp;
                                      }

                                      let new_selected = [...this.state.selected];

                                      for(let j=first; j<=last;j++) {
                                          let client_id = this.state.clients[j].client_id;
                                          if (!_.includes(new_selected, client_id)) {
                                              new_selected.push(client_id);
                                          }
                                      }

                                      this.setState({selected: new_selected});
                                      return;
                                  }

                                  this.setState({selected_idx: i});
                                  if(is_selected) {
                                      let new_selected = _.filter(this.state.selected,
                                                                  x=>x !== c.client_id );
                                      this.setState({
                                          selected: new_selected,
                                          selected_icon: new_selected.length ?
                                              "square-minus" : "square",
                                      });

                                  } else {
                                      let new_selected = [...this.state.selected, client_id];
                                      this.setState({
                                          selected: new_selected,
                                          selected_icon: new_selected.length === num_clients ?
                                              "square-check" : "square-minus" ,
                                      });
                                  }
                              }}>
                            <td>
                              <Button variant="outline-default">
                                <span className="button-label checkbox">
                                  <FontAwesomeIcon icon={
                                      ["far", this.isSelected(c) ?
                                          "square-check" : "square"]
                                  }/>
                                </span>
                              </Button>
                            </td>
                            <td>
                              <Button variant="outline-default"
                                      className="online-status">
                                <span className="button-label">
                                  <VeloClientStatusIcon client={c}/>
                                </span>
                              </Button>
                            </td>
                            <td><ClientLink client_id={c && c.client_id}/></td>
                            <td>{c && c.os_info && c.os_info.hostname}</td>
                            <td>{c && c.os_info && c.os_info.fqdn}</td>
                            <td>{c && c.os_info && c.os_info.release}</td>
                            <td>{_.map(c.labels, (label, idx)=>{
                                return <ButtonGroup key={idx}>
                                         <Button size="sm"
                                                 onClick={() => this.searchLabel(label, c)}
                                                 variant="default">
                                           <span className="button-label">{label}</span>
                                         </Button>
                                         <Button size="sm"
                                                 onClick={() => this.removeLabel(label, c)}
                                                 variant="default">
                                           <FontAwesomeIcon icon="window-close"/>
                                         </Button>
                                       </ButtonGroup>;
                            })}</td>
                          </tr>);
                  })}
                </tbody>
              </Table>
            </>);
    }

    render() {
        return (
            <>
              { this.renderToolbar() }
              <div className="fill-parent no-margins toolbar-margin selectable">
                { this.renderBasicTable() }
                </div>
              </>
        );
    }
};


export default withRouter(VeloClientList);
